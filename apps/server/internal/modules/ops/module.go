// Package ops 运维模块（R0）：Outbox 死信查询与重放（p1 §6.2；p6 降级演练复用）。
// 只做后台运维操作，不承载业务。
package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/featureflag"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/obs"
	"catcube/internal/kernel/rbac"
	"catcube/internal/kernel/scheduler"
	"catcube/internal/kernel/settings"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/ports"
	"catcube/pkg/ids"
)

// JobRunner 任务运行器（scheduler.Runner 的结构化接口，避免 ops 直接依赖 scheduler 内部）。
type JobRunner interface {
	Jobs() []scheduler.Job
	RunOnce(ctx context.Context, name string) (bool, error)
}

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Storage  ports.Storage
	Runner   JobRunner
	Audit    *audit.Recorder
	Identity idcontract.Facade
	Outbox   eventbus.OutboxStore
	Bus      *eventbus.Bus
	Flags    *featureflag.Store
	Settings *settings.Registry
	RBAC     *rbac.Store
}

// Module ops 模块。
type Module struct {
	deps Deps
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "ops" }
func (m *Module) Rank() module.Rank   { return module.RankR1 }
func (m *Module) DependsOn() []string { return []string{"identity"} }

func (m *Module) Register(r *module.Registry) error {
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/dead-letters", Handler: m.auth(m.handleList), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/dead-letters/{eventID}/replay", Handler: m.auth(m.handleReplay), Admin: true},
		// 任务重跑（代操作清单，p4 §4.4）
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/jobs", Handler: m.auth(m.handleJobs), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/jobs/{name}/run", Handler: m.auth(m.handleRunJob), Admin: true},
		// 导出中心（p4 §7.4）：申请 → 异步生成 → 下载（带水印 + 导出审计）
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/exports", Handler: m.auth(m.handleCreateExport), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/exports", Handler: m.auth(m.handleListExports), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/exports/{id}/download", Handler: m.auth(m.handleDownloadExport), Admin: true},
		// 小程序埋点（p4 §7.2：独立 tlm_ 存储，仅登记事件可入库）
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/telemetry", Handler: m.auth(m.handleTelemetry)},
		// 数据概览（p4 §7.1：12 项核心指标）
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/dashboard", Handler: m.auth(m.handleDashboard), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/dashboard/funnel", Handler: m.auth(m.handleFunnel), Admin: true},
		// 审计查询（治理：who/where/when/what/result 五维筛选；p4 §5.5）
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/audits", Handler: m.auth(m.handleAudits), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/audits/{id}", Handler: m.auth(m.handleAuditDetail), Admin: true},
		// 业务配置（配置三层第三层：后台改，热生效）
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/settings", Handler: m.auth(m.handleSettings), Admin: true, Perm: "ops:settings:read"},
		module.Route{Method: http.MethodPut, Path: "/api/admin/v1/settings/{key}", Handler: m.auth(m.handleSetSetting), Admin: true, Perm: "ops:settings:set"},
		// 特性开关（灰度发布）：后台配置 + 客户端评估
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/feature-flags", Handler: m.auth(m.handleFlags), Admin: true, Perm: "ops:settings:read"},
		module.Route{Method: http.MethodPut, Path: "/api/admin/v1/feature-flags/{key}", Handler: m.auth(m.handleSetFlag), Admin: true, Perm: "ops:settings:set"},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/flags", Handler: m.auth(m.handleMyFlags)},
	)
	r.Settings = append(r.Settings, Schemas...)
	r.Jobs = append(r.Jobs,
		// 导出生成（worker）
		module.JobDef{Name: "ops.export_runner", Interval: 10 * time.Second, Run: m.exportRunner},
		// 安全监控：denied 突增告警（p4 §5.6）
		module.JobDef{Name: "ops.security_monitor", Interval: time.Minute, Run: m.securityMonitor},
		// 审计留存归档（p4 §5.7）：超期行标记归档 + 边界哈希
		module.JobDef{Name: "ops.audit_archive", Interval: 6 * time.Hour, Run: m.archiveAudits},
	)
	r.AuditPoints = append(r.AuditPoints,
		audit.Point{Action: "ops:dead_letter:replay", Path: "/api/admin/v1/dead-letters/{eventID}/replay", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "ops:feature_flag:set", Path: "/api/admin/v1/feature-flags/{key}", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "ops:setting:set", Path: "/api/admin/v1/settings/{key}", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "ops:job:run", Path: "/api/admin/v1/jobs/{name}/run", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "export:create", Path: "/api/admin/v1/exports", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "export:download", Path: "/api/admin/v1/exports/{id}/download", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "channel:commission_rule:set", Path: "/api/admin/v1/settings/{key}", Required: true, LogFailed: true, Module: "channel"},
		audit.Point{Action: "discovery:weight:set", Path: "/api/admin/v1/settings/{key}", Required: true, LogFailed: true, Module: "discovery"},
		audit.Point{Action: "membership:price:set", Path: "/api/admin/v1/settings/{key}", Required: true, LogFailed: true, Module: "membership"},
		audit.Point{Action: "ops:miniapp:set", Path: "/api/admin/v1/settings/{key}", Required: true, LogFailed: true, Module: "ops"},
	)
	return nil
}

func (m *Module) handleList(w http.ResponseWriter, req *http.Request) {
	events, err := m.deps.Outbox.ListDead(req.Context())
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	items := make([]map[string]any, 0, len(events))
	for _, e := range events {
		items = append(items, map[string]any{
			"event_id": e.ID, "topic": e.Topic, "space_id": e.SpaceID,
			"occurred_at": e.OccurredAt, "payload": json.RawMessage(e.Payload),
		})
	}
	writeData(w, map[string]any{"items": items})
}

// Schemas 治理配置声明。
var Schemas = []settings.Schema{
	{Key: "audit.retention_days", Type: settings.TypeInt, Default: "180", Description: "审计留存天数（资金/合规类建议 ≥1095）"},
	{Key: "audit.denied_surge_threshold", Type: settings.TypeInt, Default: "50", Description: "5 分钟内 denied 审计告警阈值"},

	// 系统设置 · 小程序接入（超级管理员；热生效）
	{Key: "miniapp.appid", Type: settings.TypeString, Default: "", Description: "小程序 AppID（留空回退环境变量）"},
	{Key: "miniapp.secret", Type: settings.TypeString, Default: "", Description: "小程序 AppSecret（敏感；仅超级管理员可改）", Sensitive: true},
	{Key: "miniapp.env_version", Type: settings.TypeString, Default: "release", Description: "小程序码环境版本（release/trial/develop）"},

	// 系统设置 · 登录安全（p4 §3.4；超管/运营管理员在后台「系统设置」配置）
	{Key: "admin.security.login_fail_threshold", Type: settings.TypeInt, Default: "3",
		Description: "后台账号连续登录失败达到该次数触发短信二次验证"},
	{Key: "admin.security.sms_2fa_enabled", Type: settings.TypeBool, Default: "true",
		Description: "是否启用短信二次验证（登录失败触发）"},
	{Key: "admin.security.ip_allowlist", Type: settings.TypeString, Default: "",
		Description: "后台 IP 白名单（逗号分隔，支持前缀如 10.0.0.；空=不限制）"},

	// 系统设置 · 密码策略（独立后台账号启用后生效，预留）
	{Key: "admin.security.password_min_len", Type: settings.TypeInt, Default: "8", Description: "后台密码最小长度"},
	{Key: "admin.security.password_policy", Type: settings.TypeString, Default: "upper,lower,digit",
		Description: "后台密码复杂度（逗号分隔：upper/lower/digit/symbol）"},

	// 系统设置 · 短信网关
	{Key: "sms.gateway.provider", Type: settings.TypeString, Default: "dev", Description: "短信网关供应商（dev=日志模拟）"},
	{Key: "sms.gateway.endpoint", Type: settings.TypeString, Default: "", Description: "短信网关接入地址"},
	{Key: "sms.gateway.sign", Type: settings.TypeString, Default: "", Description: "短信签名"},

	// 系统设置 · AI 网关（ADR-003）
	{Key: "ai.gateway.enabled", Type: settings.TypeBool, Default: "false", Description: "是否启用独立 AI 网关（安全围栏）"},
	{Key: "ai.gateway.url", Type: settings.TypeString, Default: "", Description: "AI 网关接入地址"},
	{Key: "ai.gateway.key", Type: settings.TypeString, Default: "", Description: "AI 网关密钥（敏感；仅超管可见/修改）", Sensitive: true},

	// 系统设置 · 复核策略（p4 §3.12）：复核 ≠ 执行
	{Key: "policy.dual_control", Type: settings.TypeString,
		Default:     "trade:refund,channel:settlement,membership:adjust,channel:commission_rule,ops:export",
		Description: "强制双人复核（复核≠执行）的操作清单（逗号分隔）；off=关闭（资金类不建议）"},
}

// handleAudits 审计查询（五维筛选）。
func (m *Module) handleAudits(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{}
	where := []string{"1=1"}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if v := q.Get("actor_id"); v != "" {
		add("actor_id=$%d", v)
	}
	if v := q.Get("action"); v != "" {
		add("action=$%d", v)
	}
	if v := q.Get("result"); v != "" {
		add("result=$%d", v)
	}
	if v := q.Get("target_ref"); v != "" {
		add("target_ref::text LIKE '%%'||$%d||'%%'", v)
	}
	if v := q.Get("from"); v != "" {
		add("created_at >= to_timestamp($%d::bigint)", v)
	}
	if v := q.Get("to"); v != "" {
		add("created_at < to_timestamp($%d::bigint)", v)
	}
	args = append(args, limit)
	sql := fmt.Sprintf(`
		SELECT id, actor_id, actor_type, channel, action, COALESCE(target_ref::text,''), detail, result,
		       COALESCE(host(ip),''), COALESCE(user_agent,''), COALESCE(request_id,''), btrim(hash),
		       EXTRACT(EPOCH FROM created_at)::bigint, archived_at IS NOT NULL
		FROM sys_audit WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`, strings.Join(where, " AND "), len(args))
	rows, err := m.deps.Pool.Query(req.Context(), sql, args...)
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, actorID, createdAt int64
		var actorType, result int16
		var channel, action, target, ua, rid, hash string
		var detail []byte
		var archived bool
		var ip string
		if err := rows.Scan(&id, &actorID, &actorType, &channel, &action, &target, &detail, &result,
			&ip, &ua, &rid, &hash, &createdAt, &archived); err != nil {
			writeErr(w, 9, "服务错误")
			return
		}
		items = append(items, auditRow(id, actorID, actorType, channel, action, target, detail, result, ip, rid, hash, createdAt, archived,
			m.actorName(req, actorID), auditActionLabel(action)))
	}
	writeData(w, map[string]any{"items": items})
}

// handleAuditDetail 单条审计详情（含前后值快照字段与哈希）。
func (m *Module) handleAuditDetail(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if id == 0 {
		writeErr(w, 1, "参数错误")
		return
	}
	var actorID, createdAt int64
	var actorType, result int16
	var channel, action, target, ua, rid, hash, prevHash, ip string
	var detail []byte
	var archived bool
	err := m.deps.Pool.QueryRow(req.Context(), `
		SELECT id, actor_id, actor_type, channel, action, COALESCE(target_ref::text,''), detail, result,
		       COALESCE(host(ip),''), COALESCE(user_agent,''), COALESCE(request_id,''), btrim(hash), btrim(prev_hash),
		       EXTRACT(EPOCH FROM created_at)::bigint, archived_at IS NOT NULL
		FROM sys_audit WHERE id=$1`, id).
		Scan(&id, &actorID, &actorType, &channel, &action, &target, &detail, &result, &ip, &ua, &rid, &hash, &prevHash, &createdAt, &archived)
	if err != nil {
		writeErr(w, 4, "审计记录不存在")
		return
	}
	row := auditRow(id, actorID, actorType, channel, action, target, detail, result, ip, rid, hash, createdAt, archived,
		m.actorName(req, actorID), auditActionLabel(action))
	row["prev_hash"] = prevHash
	writeData(w, row)
}

func auditRow(id, actorID int64, actorType int16, channel, action, target string, detail []byte, result int16,
	ip, rid, hash string, createdAt int64, archived bool, actorName, actionLabel string) map[string]any {
	var dv any
	if len(detail) > 0 {
		var v any
		if json.Unmarshal(detail, &v) == nil {
			dv = audit.SanitizeDetail(v) // 二次脱敏（存量数据兜底）
		}
	}
	return map[string]any{
		"id": id, "actor_id": strconv.FormatInt(actorID, 10), "actor_name": actorName, "actor_type": actorType,
		"channel": channel, "action": action, "action_label": actionLabel,
		"target_ref": target, "detail": dv, "result": result, "ip": ip, "request_id": rid,
		"hash": hash, "created_at": createdAt, "archived": archived,
	}
}

// actorName 操作者姓名（系统/后台/小程序统一经 identity facade 取昵称）。
func (m *Module) actorName(req *http.Request, actorID int64) string {
	if actorID == 0 {
		return "系统"
	}
	if m.deps.Identity == nil {
		return ""
	}
	b, err := m.deps.Identity.UserBrief(req.Context(), actorID)
	if err != nil {
		return ""
	}
	return b.Nickname
}

// securityMonitor denied 突增告警（p4 §5.6）。
func (m *Module) securityMonitor(ctx context.Context) error {
	threshold := int64(50)
	if m.deps.Settings != nil {
		if v, err := m.deps.Settings.Int(ctx, 1, "audit.denied_surge_threshold"); err == nil {
			threshold = v
		}
	}
	var denied int64
	if err := m.deps.Pool.QueryRow(ctx, `
		SELECT count(1) FROM sys_audit WHERE result=1 AND created_at > now() - interval '5 minutes'`).Scan(&denied); err != nil {
		return err
	}
	obs.Default().SetGauge("catcube_audit_denied_5m", float64(denied))
	if threshold > 0 && denied > threshold {
		obs.Default().Alert("audit_denied_surge", fmt.Sprintf("5 分钟内 denied %d 超过阈值 %d", denied, threshold))
	}
	return nil
}

// archiveAudits 审计留存归档：超期行标记 archived_at 并记录边界哈希（不物理删除，链可续验）。
func (m *Module) archiveAudits(ctx context.Context) error {
	days := int64(180)
	if m.deps.Settings != nil {
		if v, err := m.deps.Settings.Int(ctx, 1, "audit.retention_days"); err == nil && v >= 0 {
			days = v // 0 = 立即归档（演练用）
		}
	}
	var lastID int64
	var boundary string
	var n int64
	err := m.deps.Pool.QueryRow(ctx, `
		SELECT COALESCE(max(id),0) FROM sys_audit WHERE created_at < now() - ($1 || ' days')::interval AND archived_at IS NULL`,
		strconv.FormatInt(days, 10)).Scan(&lastID)
	if err != nil || lastID == 0 {
		return err
	}
	if err := m.deps.Pool.QueryRow(ctx, `SELECT btrim(hash) FROM sys_audit WHERE id=$1`, lastID).Scan(&boundary); err != nil {
		return err
	}
	tag, err := m.deps.Pool.Exec(ctx, `
		UPDATE sys_audit SET archived_at=now()
		WHERE id <= $1 AND created_at < now() - ($2 || ' days')::interval AND archived_at IS NULL`,
		lastID, strconv.FormatInt(days, 10))
	if err != nil {
		return err
	}
	n = tag.RowsAffected()
	if n == 0 {
		return nil
	}
	if _, err := m.deps.Pool.Exec(ctx, `
		INSERT INTO sys_audit_archive (id, boundary_hash, last_audit_id, rows_archived, archived_at)
		VALUES ($1,$2,$3,$4,now())`, int64(ids.New()), boundary, lastID, n); err != nil {
		return err
	}
	obs.Default().Add("catcube_audit_archived_total", float64(n))
	return nil
}

// handleSettings 后台：业务配置列表（含声明与当前值）。
func (m *Module) handleSettings(w http.ResponseWriter, req *http.Request) {
	if m.deps.Settings == nil {
		writeData(w, map[string]any{"items": []any{}})
		return
	}
	items := make([]map[string]any, 0)
	for _, sc := range m.deps.Settings.Schemas() {
		cur, err := m.deps.Settings.Get(req.Context(), 1, sc.Key)
		value := sc.Default
		if err == nil {
			value = cur
		}
		if sc.Sensitive && value != "" {
			value = "******" // 敏感配置不回显明文
		}
		items = append(items, map[string]any{
			"key": sc.Key, "type": string(sc.Type), "value": value,
			"default": sc.Default, "description": sc.Description, "sensitive": sc.Sensitive,
		})
	}
	writeData(w, map[string]any{"items": items})
}

// handleSetSetting 后台：修改业务配置（热生效；敏感项不回显明文）。
func (m *Module) handleSetSetting(w http.ResponseWriter, req *http.Request) {
	key := req.PathValue("key")
	if key == "" || m.deps.Settings == nil {
		writeErr(w, 1, "参数错误")
		return
	}
	if _, err := m.deps.Settings.GetSchema(key); err != nil {
		writeErr(w, 1, "未登记的配置项")
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, 1, "参数错误")
		return
	}
	op := userIDFrom(req.Context())
	if strings.HasPrefix(key, "miniapp.") {
		super, err := m.deps.RBAC.IsSuper(req.Context(), op)
		if err != nil || !super {
			writeErr(w, 3, "仅超级管理员可修改小程序设置")
			return
		}
	}
	before, _ := m.deps.Settings.Get(req.Context(), 1, key)
	sc, _ := m.deps.Settings.GetSchema(key)
	if err := m.deps.Settings.Set(req.Context(), 1, key, body.Value, op); err != nil {
		m.recordSetting(req, op, key, sc, before, body.Value, audit.ResultFailed)
		writeErr(w, 9, "服务错误")
		return
	}
	m.recordSetting(req, op, key, sc, before, body.Value, audit.ResultOK)
	writeData(w, map[string]any{"saved": true})
}

// recordSetting 配置中心审计：佣金规则/匹配权重等按业务归属记独立 action（p4 §5.4）。
func (m *Module) recordSetting(req *http.Request, op int64, key string, sc settings.Schema, before, after string, result audit.Result) {
	if m.deps.Audit == nil {
		return
	}
	// 敏感值不落审计原文
	redact := func(v string) string {
		if sc.Sensitive && v != "" && v != "******" {
			return "******"
		}
		return v
	}
	detail := audit.Change{Before: map[string]any{"value": redact(before)}, After: map[string]any{"value": redact(after)}}.Snapshot(key)
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: op, ActorType: audit.ActorAdmin, Channel: "admin", Action: SettingAction(key),
		TargetRef: `{"type":"setting","id":"` + key + `"}`, Detail: detail,
		Result: result, IP: audit.ClientIP(req), RequestID: audit.RequestIDOf(req.Context()),
	})
}

// SettingAction 配置键 → 审计动作（跨模块配置归属其业务模块）。
func SettingAction(key string) string {
	switch {
	case strings.HasPrefix(key, "channel.commission."):
		return "channel:commission_rule:set"
	case strings.HasPrefix(key, "dsc.weight."):
		return "discovery:weight:set"
	case strings.HasPrefix(key, "membership.year_fee_"), strings.HasPrefix(key, "membership.discount_"):
		return "membership:price:set"
	case strings.HasPrefix(key, "miniapp."):
		return "ops:miniapp:set"
	default:
		return "ops:setting:set"
	}
}

// handleFlags 后台：开关列表。
func (m *Module) handleFlags(w http.ResponseWriter, req *http.Request) {
	if m.deps.Flags == nil {
		writeData(w, map[string]any{"items": []any{}})
		return
	}
	list, err := m.deps.Flags.List(req.Context())
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, f := range list {
		items = append(items, map[string]any{"key": f.Key, "enabled": f.Enabled,
			"ratio": f.Rules.Ratio, "users": int64Strings(f.Rules.Users)})
	}
	writeData(w, map[string]any{"items": items, "known": featureflag.Known})
}

// handleSetFlag 后台：配置开关（enabled/ratio/users）。
func (m *Module) handleSetFlag(w http.ResponseWriter, req *http.Request) {
	key := req.PathValue("key")
	if key == "" || m.deps.Flags == nil {
		writeErr(w, 1, "参数错误")
		return
	}
	var body struct {
		Enabled bool              `json:"enabled"`
		Ratio   int               `json:"ratio"`
		Users   []json.RawMessage `json:"users"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, 1, "参数错误")
		return
	}
	users := parseUserIDs(body.Users)
	if body.Ratio < 0 || body.Ratio > 100 {
		writeErr(w, 1, "ratio 需在 0-100")
		return
	}
	var before map[string]any
	if list, err := m.deps.Flags.List(req.Context()); err == nil {
		for _, f := range list {
			if f.Key == key {
				before = map[string]any{"enabled": f.Enabled, "ratio": f.Rules.Ratio, "users": int64Strings(f.Rules.Users)}
			}
		}
	}
	err := m.deps.Flags.Set(req.Context(), featureflag.Flag{Key: key, Enabled: body.Enabled,
		Rules: featureflag.Rules{Users: users, Ratio: body.Ratio}})
	if m.deps.Audit != nil {
		res := audit.ResultOK
		if err != nil {
			res = audit.ResultFailed
		}
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin",
			Action: "ops:feature_flag:set", TargetRef: `{"type":"feature_flag","id":"` + key + `"}`,
			Detail: audit.Change{Before: before, After: map[string]any{"enabled": body.Enabled,
				"ratio": body.Ratio, "users": int64Strings(users)}}.Snapshot("特性开关切换"),
			Result: res, IP: audit.ClientIP(req), RequestID: audit.RequestIDOf(req.Context()),
		})
	}
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	writeData(w, map[string]any{"saved": true})
}

// handleMyFlags 客户端：按当前用户评估全部开关（灰度放量入口）。
func (m *Module) handleMyFlags(w http.ResponseWriter, req *http.Request) {
	out := map[string]bool{}
	if m.deps.Flags != nil {
		if list, err := m.deps.Flags.List(req.Context()); err == nil {
			uid := userIDFrom(req.Context())
			for _, f := range list {
				out[f.Key] = featureflag.Eval(f, uid)
			}
		}
	}
	writeData(w, map[string]any{"flags": out})
}

func (m *Module) handleReplay(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("eventID")
	if id == "" {
		writeErr(w, 1, "参数错误")
		return
	}
	err := m.deps.Outbox.Replay(req.Context(), id)
	if m.deps.Audit != nil {
		res := audit.ResultOK
		if err != nil {
			res = audit.ResultFailed
		}
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin",
			Action: "ops:dead_letter:replay", TargetRef: `{"type":"dead_letter","id":"` + id + `"}`,
			Detail: audit.Change{Before: map[string]any{"state": "dead"},
				After: map[string]any{"state": "replayed", "error": errText(err)}}.Snapshot("死信重放"),
			Result: res, IP: audit.ClientIP(req), RequestID: audit.RequestIDOf(req.Context()),
		})
	}
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	writeData(w, map[string]any{"replayed": true})
}

func (m *Module) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			writeErr(w, 2, "未登录")
			return
		}
		uid, ver, err := m.deps.Identity.Authenticate(req.Context(), token)
		if err != nil {
			writeErr(w, 2, "登录已失效")
			return
		}
		brief, err := m.deps.Identity.UserBrief(req.Context(), uid)
		if err != nil || brief.AuthzVer != ver || (brief.Status != 0 && brief.Status != 1) {
			writeErr(w, 2, "登录已失效")
			return
		}
		next(w, req.WithContext(context.WithValue(req.Context(), ctxUserID{}, uid)))
	}
}

type ctxUserID struct{}

func userIDFrom(ctx context.Context) int64 {
	if v, ok := ctx.Value(ctxUserID{}).(int64); ok {
		return v
	}
	return 0
}

func bearer(req *http.Request) string {
	// 后台经 Nginx Basic Auth 时 Authorization 被占用，令牌可由 X-Admin-Token 传入
	if t := strings.TrimSpace(req.Header.Get("X-Admin-Token")); t != "" {
		return t
	}
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

func writeErr(w http.ResponseWriter, code int, message string) {
	status := http.StatusInternalServerError
	switch code {
	case 1:
		status = http.StatusBadRequest
	case 2:
		status = http.StatusUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}

// ---- 通用小工具（导出/埋点共用）----

func decodeJSON(req *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(req.Body, 1<<20)).Decode(v)
}

func jsonUnmarshal(raw []byte, v any) error { return json.Unmarshal(raw, v) }

// replayAudit 审计写入器（可空）。
func (m *Module) replayAudit() *audit.Recorder { return m.deps.Audit }

// logErr 记录运维错误（排障用）。
func logErr(action string, err error) {
	if err != nil {
		slog.Error("ops: "+action, "err", err)
	}
}

// int64Strings 用户 ID 以字符串返回，避免 JS Number 精度丢失。
func int64Strings(ids []int64) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, strconv.FormatInt(id, 10))
	}
	return out
}

// parseUserIDs 兼容前端字符串与旧脚本数字。
func parseUserIDs(raws []json.RawMessage) []int64 {
	out := make([]int64, 0, len(raws))
	for _, raw := range raws {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if id, err := strconv.ParseInt(s, 10, 64); err == nil {
				out = append(out, id)
			}
			continue
		}
		var n int64
		if json.Unmarshal(raw, &n) == nil {
			out = append(out, n)
		}
	}
	return out
}
