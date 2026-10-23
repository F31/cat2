// Package notice R6 通知中心：模板数据驱动（事件→接收人→文案→渠道→合并），业务模块从不直接发通知。
package notice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"catcube/internal/ports"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/settings"
	circlecontract "catcube/internal/modules/circle/contract"
	idcontract "catcube/internal/modules/identity/contract"
	noticecontract "catcube/internal/modules/notice/contract"
	"catcube/internal/modules/notice/internal/app"
	"catcube/internal/modules/notice/internal/domain"
	"catcube/internal/modules/notice/internal/infra"
)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Identity idcontract.Facade
	Circle   circlecontract.Facade
	Audit    *audit.Recorder
	Notifier ports.Notify
	Settings *settings.Registry
}

// Module notice 模块。
type Module struct {
	deps  Deps
	svc   *app.Service
	fault atomic.Value // func(eventbus.Event) error（降级演练：模拟通知通道故障）
}

// SetFault 注入故障（演练/测试）：非 nil 时通知处理前先返回该错误，业务不受影响、
// 事件保持积压，故障解除后可重放补齐（p6 §4.5）。传 nil 解除。
func (m *Module) SetFault(f func(eventbus.Event) error) { m.fault.Store(faultFn(f)) }

type faultFn func(eventbus.Event) error

// handleEvent 订阅包装：故障注入 → 正常处理。
func (m *Module) handleEvent(ctx context.Context, e eventbus.Event) error {
	if v := m.fault.Load(); v != nil {
		if f, ok := v.(faultFn); ok && f != nil {
			return f(e)
		}
	}
	return m.svc.Handle(ctx, e)
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "notice" }
func (m *Module) Rank() module.Rank   { return module.RankR6 }
func (m *Module) DependsOn() []string { return []string{"identity", "circle"} }

// notice 错误码（contracts/errors/README.md 17 段）。
const (
	codeParamInvalid = 1
	codeUnauthorized = 2
	codeInternal     = 9
	codeNotFound     = 170001
	codeQuotaShort   = 170002
	codeTemplate     = 170003
)

func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{
		Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus, Circle: m.deps.Circle,
		Subscribe: m.deps.Notifier, OpenID: m.deps.Identity.OpenID,
		TemplateID: func(ctx context.Context, code string) (string, bool) {
			if m.deps.Settings == nil {
				return "", false
			}
			raw, err := m.deps.Settings.Get(ctx, 1, "notice.wechat_templates")
			if err != nil {
				return "", false
			}
			return wechatTemplateID(raw, code)
		},
	})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/notices", Handler: m.auth(m.handleList)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/notices/{id}/read", Handler: m.auth(m.handleRead)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/notice-preferences", Handler: m.auth(m.handlePreferences)},
		module.Route{Method: http.MethodPut, Path: "/api/mp/v1/notice-preferences", Handler: m.auth(m.handleSetPreference)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/subscribe-quota/apply", Handler: m.auth(m.handleApplyQuota)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/subscribe-templates", Handler: m.auth(m.handleSubscribeTemplates)},
		// 后台：模板与公告管理（页面随 p4；此处提供只读列表）
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/notice-templates", Handler: m.auth(m.handleAdminTemplates), Admin: true, Perm: "notice:announcement:broadcast"},
		module.Route{Method: http.MethodPut, Path: "/api/admin/v1/notice-templates/{code}", Handler: m.auth(m.handleAdminTemplateUpdate), Admin: true, Perm: "notice:announcement:broadcast"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/notices/broadcast", Handler: m.auth(m.handleAdminBroadcast), Admin: true, Perm: "notice:announcement:broadcast"},
	)
	// 订阅全部业务事件（事件驱动触达）
	for _, topic := range app.Topics() {
		r.EventSubs = append(r.EventSubs, eventbus.Subscription{Topic: topic, Handler: m.handleEvent})
	}
	r.Jobs = append(r.Jobs,
		module.JobDef{Name: "notice.seed_templates", Interval: 10 * time.Minute, Run: m.svc.Register},
		module.JobDef{Name: "notice.digest", Interval: time.Minute, Run: m.svc.RunDigest},
	)
	r.AuditPoints = append(r.AuditPoints,
		audit.Point{Action: "notice:announcement:broadcast", Path: "/api/admin/v1/notices/broadcast", Required: true, LogFailed: true, Module: "notice"},
		audit.Point{Action: "notice:template:update", Path: "/api/admin/v1/notice-templates/{code}", Required: true, LogFailed: true, Module: "notice"},
	)
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() noticecontract.Facade { return m }

func (m *Module) List(ctx context.Context, userID int64, unreadOnly bool, limit int) ([]noticecontract.Notice, error) {
	return m.svc.List(ctx, userID, unreadOnly, limit)
}
func (m *Module) UnreadCount(ctx context.Context, userID int64) (int, error) {
	return m.svc.UnreadCount(ctx, userID)
}
func (m *Module) MarkRead(ctx context.Context, userID, noticeID int64) error {
	return m.svc.MarkRead(ctx, userID, noticeID)
}
func (m *Module) Preferences(ctx context.Context, userID int64) ([]noticecontract.Preference, error) {
	return m.svc.Preferences(ctx, userID)
}
func (m *Module) SetPreference(ctx context.Context, userID int64, code string, enabled bool) error {
	return m.svc.SetPreference(ctx, userID, code, enabled)
}
func (m *Module) ApplyQuota(ctx context.Context, userID int64, code string, n int) error {
	return m.svc.ApplyQuota(ctx, userID, code, n)
}

// RunDigest 手动触发日结（运维入口）。
func (m *Module) RunDigest(ctx context.Context) error { return m.svc.RunDigest(ctx) }

// SeedTemplates 手动登记模板（运维入口；定时由 notice.seed_templates Job 调用）。
func (m *Module) SeedTemplates(ctx context.Context) error { return m.svc.Register(ctx) }

// ---- HTTP ----

func (m *Module) handleList(w http.ResponseWriter, req *http.Request) {
	unreadOnly := req.URL.Query().Get("unread") == "1"
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	items, err := m.svc.List(req.Context(), userIDFrom(req.Context()), unreadOnly, limit)
	if err != nil {
		writeNoticeErr(w, err)
		return
	}
	unread, _ := m.svc.UnreadCount(req.Context(), userIDFrom(req.Context()))
	writeData(w, map[string]any{"items": items, "unread": unread})
}

func (m *Module) handleRead(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.MarkRead(req.Context(), userIDFrom(req.Context()), id); err != nil {
		writeNoticeErr(w, err)
		return
	}
	writeData(w, map[string]any{"read": true})
}

func (m *Module) handlePreferences(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.Preferences(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeNoticeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleSetPreference(w http.ResponseWriter, req *http.Request) {
	var body struct {
		TemplateCode string `json:"template_code"`
		Enabled      bool   `json:"enabled"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.TemplateCode == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.SetPreference(req.Context(), userIDFrom(req.Context()), body.TemplateCode, body.Enabled); err != nil {
		writeNoticeErr(w, err)
		return
	}
	writeData(w, map[string]any{"saved": true})
}

func (m *Module) handleApplyQuota(w http.ResponseWriter, req *http.Request) {
	var body struct {
		TemplateCode string `json:"template_code"`
		N            int    `json:"n"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.TemplateCode == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.ApplyQuota(req.Context(), userIDFrom(req.Context()), body.TemplateCode, body.N); err != nil {
		writeNoticeErr(w, err)
		return
	}
	writeData(w, map[string]any{"applied": true})
}

// handleAdminTemplates 后台模板列表（只读；编辑页随 p4）。
func (m *Module) handleAdminTemplates(w http.ResponseWriter, req *http.Request) {
	list, err := m.svc.Templates(req.Context())
	if err != nil {
		writeNoticeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": list})
}

// handleAdminTemplateUpdate 后台修改通知模板（文案/渠道/可关闭）。
func (m *Module) handleAdminTemplateUpdate(w http.ResponseWriter, req *http.Request) {
	code := req.PathValue("code")
	var body struct {
		TitleTpl   string   `json:"title_tpl"`
		ContentTpl string   `json:"content_tpl"`
		Channels   []string `json:"channels"`
		Closeable  bool     `json:"closeable"`
	}
	if code == "" || json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	body.TitleTpl = strings.TrimSpace(body.TitleTpl)
	body.ContentTpl = strings.TrimSpace(body.ContentTpl)
	if body.TitleTpl == "" || len(body.TitleTpl) > 120 || body.ContentTpl == "" || len(body.ContentTpl) > 500 {
		writeErr(w, codeParamInvalid, "标题/内容不合法")
		return
	}
	allowed := map[string]bool{domain.ChannelInApp: true, domain.ChannelSubscribe: true, domain.ChannelSMS: true}
	channels := make([]string, 0, len(body.Channels))
	for _, c := range body.Channels {
		c = strings.TrimSpace(c)
		if !allowed[c] {
			writeErr(w, codeParamInvalid, "渠道不合法")
			return
		}
		channels = append(channels, c)
	}
	if len(channels) == 0 {
		writeErr(w, codeParamInvalid, "至少选择一个渠道")
		return
	}
	if err := m.svc.UpdateTemplate(req.Context(), code, body.TitleTpl, body.ContentTpl, channels, body.Closeable); err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			writeErr(w, codeNotFound, "模板不存在")
			return
		}
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"saved": true})
}

// ---- 辅助 ----

func (m *Module) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			writeErr(w, codeUnauthorized, "未登录")
			return
		}
		uid, ver, err := m.deps.Identity.Authenticate(req.Context(), token)
		if err != nil {
			writeErr(w, codeUnauthorized, "登录已失效")
			return
		}
		brief, err := m.deps.Identity.UserBrief(req.Context(), uid)
		if err != nil || brief.AuthzVer != ver || (brief.Status != 0 && brief.Status != 1) {
			writeErr(w, codeUnauthorized, "登录已失效")
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

func writeNoticeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeErr(w, codeNotFound, err.Error())
	case errors.Is(err, domain.ErrTemplateNotFound):
		writeErr(w, codeTemplate, err.Error())
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeNotFound, codeTemplate:
		return http.StatusNotFound
	case codeParamInvalid:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

func writeErr(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus(code))
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}

var _ noticecontract.Facade = (*Module)(nil)

// handleAdminBroadcast 定向公告（管理员代操作；仅站内信，审计留痕）。
func (m *Module) handleAdminBroadcast(w http.ResponseWriter, req *http.Request) {
	var body struct {
		UserIDs []string `json:"user_ids"`
		Title   string   `json:"title"`
		Content string   `json:"content"`
		Reason  string   `json:"reason"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Title == "" || body.Content == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	ids := make([]int64, 0, len(body.UserIDs))
	for _, s := range body.UserIDs {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 {
			ids = append(ids, v)
		}
	}
	if len(ids) == 0 {
		writeErr(w, codeParamInvalid, "请指定接收用户")
		return
	}
	sent, err := m.svc.Broadcast(req.Context(), ids, body.Title, body.Content)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	if m.deps.Audit != nil {
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin",
			Action: "notice:announcement:broadcast", Result: audit.ResultOK, IP: audit.ClientIP(req),
			Detail: audit.Change{
				Before: map[string]any{"targets": 0, "title": ""},
				After:  map[string]any{"targets": sent, "title": body.Title},
			}.Snapshot(body.Reason),
		})
	}
	writeData(w, map[string]any{"sent": sent})
}

// wechatTemplateID 解析后台配置：`notice.code=wxTemplateID,notice.code2=wxTemplateID2`。
func wechatTemplateID(raw, code string) (string, bool) {
	for _, part := range strings.Split(raw, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.TrimSpace(k) == code && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

// handleSubscribeTemplates 返回已配置的小程序订阅模板（客户端 wx.requestSubscribeMessage 使用）。
func (m *Module) handleSubscribeTemplates(w http.ResponseWriter, req *http.Request) {
	items := []map[string]string{}
	if m.deps.Settings != nil {
		if raw, err := m.deps.Settings.Get(req.Context(), 1, "notice.wechat_templates"); err == nil {
			for _, part := range strings.Split(raw, ",") {
				k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
				if ok && strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
					items = append(items, map[string]string{"code": strings.TrimSpace(k), "template_id": strings.TrimSpace(v)})
				}
			}
		}
	}
	writeData(w, map[string]any{"items": items})
}
