// Package trust R0 内容安全与审核模块：敏感词前置过滤、文本同步机检、统一审核队列、
// 举报复核与违规记分、账号限制；ReviewSubjectProvider 注册表统一「取素材/落结论」。
package trust

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/obs"
	"catcube/internal/modules/trust/contract"
	"catcube/internal/ports"
	"catcube/pkg/ids"
)

// AuthFunc 鉴权函数（由装配根注入 identity 校验，避免 R0 同阶依赖）。
type AuthFunc func(ctx context.Context, accessToken string) (userID int64, err error)

// Deps 装配依赖。
type Deps struct {
	Pool       *db.Pool
	Moderation ports.Moderation
	Bus        *eventbus.Bus
	Auth       AuthFunc
	Audit      *audit.Recorder
	// AlertReviewBacklog 审核积压告警阈值（0=不告警）
	AlertReviewBacklog int
}

// Module trust 模块。
type Module struct {
	deps Deps

	mu        sync.RWMutex
	providers map[string]contract.ReviewSubjectProvider
	words     []string
}

func New(deps Deps) *Module {
	return &Module{deps: deps, providers: map[string]contract.ReviewSubjectProvider{}}
}

func (m *Module) Name() string        { return "trust" }
func (m *Module) Rank() module.Rank   { return module.RankR0 }
func (m *Module) DependsOn() []string { return nil }

// Register 注册路由、能力提供者与审核开口。
func (m *Module) Register(r *module.Registry) error {
	r.Extensions["trust.ReviewSubjectProvider"] = func(p contract.ReviewSubjectProvider) {
		m.RegisterSubjectProvider(p)
	}
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/report-reasons", Handler: m.handleReasons},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/reports", Handler: m.auth(m.handleReport)},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/review-tasks", Handler: m.auth(m.handleQueue), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/review-tasks/{id}/resolve", Handler: m.auth(m.handleResolve), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/review-metrics", Handler: m.auth(m.handleMetrics), Admin: true},
	)
	r.Jobs = append(r.Jobs,
		// 审核积压与 SLA 监控（看板 + 告警，p6 §5.3/§5.4）
		module.JobDef{Name: "trust.sla_monitor", Interval: time.Minute, Run: func(ctx context.Context) error {
			met, err := m.SLAMetrics(ctx)
			if err != nil {
				return err
			}
			mm := obs.Default()
			mm.SetGauge("catcube_review_backlog", float64(met.Pending))
			mm.SetGauge("catcube_review_sla_overdue", float64(met.Overdue))
			if met.Overdue > 0 {
				mm.Alert("review_sla_overdue", fmt.Sprintf("%d 条审核任务超 SLA", met.Overdue))
			}
			if th := m.deps.AlertReviewBacklog; th > 0 && met.Pending > th {
				mm.Alert("review_backlog_high", fmt.Sprintf("审核积压 %d 超过阈值 %d", met.Pending, th))
			}
			return nil
		}},
	)
	r.CapProviders = append(r.CapProviders, capProvider{m})
	// 订阅聊天消息（msg R1 事件，topic 字面量，不 import 高阶 contract）：异步机检，命中入审核队列。
	r.EventSubs = append(r.EventSubs, eventbus.Subscription{
		Topic: "msg.MessageSent.v1", Handler: m.HandleMessageSent,
	})
	return nil
}

// HandleMessageSent 订阅 msg.MessageSent：敏感词/文本机检，命中进入人工复核队列
// （不阻塞消息投递；聊天记录留存由 msg 负责，这里只落审核任务）。
func (m *Module) HandleMessageSent(ctx context.Context, e eventbus.Event) error {
	var p struct {
		MessageID int64  `json:"message_id,string"`
		SenderID  int64  `json:"sender_id,string"`
		Content   string `json:"content"`
		MsgType   int16  `json:"msg_type"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.MessageID == 0 || p.Content == "" {
		return nil
	}
	if word, hit := m.FilterSensitive(p.Content); hit {
		return m.createTask(ctx, contract.SourceMachine, "msg.message", p.MessageID, p.SenderID, 2, "敏感词命中:"+word)
	}
	if m.deps.Moderation != nil {
		res, err := m.deps.Moderation.CheckText(ctx, ports.ModerationRequest{Text: p.Content})
		if err != nil {
			return m.createTask(ctx, contract.SourceMachine, "msg.message", p.MessageID, p.SenderID, 3, "机检超时待重试")
		}
		if !res.Pass {
			return m.createTask(ctx, contract.SourceMachine, "msg.message", p.MessageID, p.SenderID, 2, "文本机检命中")
		}
	}
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

// RegisterSubjectProvider 高阶模块注册审核对象提供者（注册即生效，删除模块即失效）。
func (m *Module) RegisterSubjectProvider(p contract.ReviewSubjectProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[p.SubjectType()] = p
}

// LoadWords 加载敏感词库（trs_sensitive_word，后台可维护）。
func (m *Module) LoadWords(ctx context.Context) error {
	rows, err := m.deps.Pool.Query(ctx, `SELECT word FROM trs_sensitive_word WHERE status=1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var words []string
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			return err
		}
		words = append(words, w)
	}
	m.mu.Lock()
	m.words = words
	m.mu.Unlock()
	return rows.Err()
}

// FilterSensitive 本地敏感词前置过滤（含引流词）；命中先提示再记分。
func (m *Module) FilterSensitive(text string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.words {
		if w != "" && strings.Contains(text, w) {
			return w, true
		}
	}
	return "", false
}

// SubmitReview 兼容接口：Pending（机检不可用）与驳回同样返回 false。
func (m *Module) SubmitReview(ctx context.Context, subjectType string, subjectID int64, text string, imageIDs []string) (bool, error) {
	o, err := m.Review(ctx, subjectType, subjectID, text, imageIDs)
	return o == contract.OutcomePassed, err
}

// Review 提交审核：敏感词前置 → 文本同步机检；结论经事件对外；高风险类目入抽检。
// 机检不可用返回 OutcomePending（内容保持「审核中」并进重试队列，不放行，p6 §4.3）。
func (m *Module) Review(ctx context.Context, subjectType string, subjectID int64, text string, imageIDs []string) (contract.Outcome, error) {
	if word, hit := m.FilterSensitive(text); hit {
		_ = m.createTask(ctx, contract.SourceMachine, subjectType, subjectID, 0, 2, "敏感词命中:"+word)
		_ = m.emit(ctx, contract.NewModerationRejected(ids.New().String(), contract.ModerationPayload{SubjectType: subjectType, SubjectID: subjectID, ReasonCode: "sensitive_word"}))
		return contract.OutcomeRejected, nil
	}
	pass := true
	if m.deps.Moderation != nil {
		res, err := m.deps.Moderation.CheckText(ctx, ports.ModerationRequest{Text: text})
		if err != nil {
			// 机检超时：进「审核中」队列重试，不放行
			_ = m.createTask(ctx, contract.SourceMachine, subjectType, subjectID, 0, 3, "机检超时待重试")
			return contract.OutcomePending, nil
		}
		pass = res.Pass
	}
	if !pass {
		_ = m.createTask(ctx, contract.SourceMachine, subjectType, subjectID, 0, 2, "文本机检命中")
		_ = m.emit(ctx, contract.NewModerationRejected(ids.New().String(), contract.ModerationPayload{SubjectType: subjectType, SubjectID: subjectID, ReasonCode: "machine_reject"}))
		return contract.OutcomeRejected, nil
	}
	// 高风险类目人工抽检（不阻塞展示）
	if cat := highRiskCategory(text); cat != "" {
		_ = m.createTask(ctx, contract.SourceHighRisk, subjectType, subjectID, 0, 3, "高风险类目抽检:"+cat)
	}
	_ = m.emit(ctx, contract.NewModerationPassed(ids.New().String(), contract.ModerationPayload{SubjectType: subjectType, SubjectID: subjectID}))
	return contract.OutcomePassed, nil
}

// Report 用户举报：写举报记录 + 入人工复核队列（24h SLA）。
func (m *Module) Report(ctx context.Context, reporterID int64, targetRef, reasonCode, detail string) error {
	if !contract.ValidReportReason(reasonCode) {
		return fmt.Errorf("trust: 非法举报原因 %q", reasonCode)
	}
	tType, tID := parseRef(targetRef)
	reportID := int64(ids.New())
	ref, _ := json.Marshal(map[string]any{"target_type": tType, "target_id": tID, "reason_code": reasonCode})
	if _, err := m.deps.Pool.Exec(ctx,
		`INSERT INTO trs_report (id, reporter_id, target_ref, reason_code, detail, status, created_at)
		 VALUES ($1,$2,$3,$4,$5,0,now())`,
		reportID, reporterID, ref, reasonCode, detail); err != nil {
		return err
	}
	return m.createTask(ctx, contract.SourceReport, tType, tID, reporterID, 2, "举报:"+reasonCode)
}

// Restrict 限制/封禁并发布 AccountRestricted。
func (m *Module) Restrict(ctx context.Context, userID int64, days int, reason string) error {
	typ := int16(1)
	if days <= 0 {
		typ = 2 // days<=0 视为封禁
		days = 3650
	}
	if _, err := m.deps.Pool.Exec(ctx,
		`INSERT INTO trs_restriction (id, user_id, type, days, reason, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,now())`,
		int64(ids.New()), userID, typ, days, reason, time.Now().Add(time.Duration(days)*24*time.Hour)); err != nil {
		return err
	}
	return m.emit(ctx, contract.NewAccountRestricted(ids.New().String(), contract.AccountRestrictedPayload{
		UserID: userID, Type: typ, Days: days, Reason: reason,
	}))
}

// IsRestricted 是否被限制发布。
func (m *Module) IsRestricted(ctx context.Context, userID int64) (bool, error) {
	var n int
	err := m.deps.Pool.QueryRow(ctx,
		`SELECT count(1) FROM trs_restriction WHERE user_id=$1 AND expires_at>now() AND lifted_at IS NULL`, userID).Scan(&n)
	return n > 0, err
}

// ResolveReview 后台复核：属实 → 驳回对象 + 记分 + 3 次限制发布 7 天。
func (m *Module) ResolveReview(ctx context.Context, taskID, reviewerID int64, confirmed bool, reason string) (contract.Verdict, error) {
	task, err := m.loadTask(ctx, taskID)
	if err != nil {
		return contract.Verdict{}, err
	}
	if task.Status != 0 {
		return contract.Verdict{}, fmt.Errorf("trust: 审核任务已处理")
	}
	v := contract.Verdict{TaskID: taskID, SubjectType: task.SubjectType, SubjectID: task.SubjectID}

	if p := m.provider(task.SubjectType); p != nil {
		if owner, e := p.OwnerOf(ctx, task.SubjectID); e == nil {
			v.UserID = owner
		}
		_ = p.ApplyVerdict(ctx, task.SubjectID, !confirmed, reason)
	}

	newStatus := int16(1) // 通过
	if confirmed {
		newStatus = 2 // 属实/驳回对象
	}
	if err := m.pool().InTx(ctx, func(tx db.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE trs_review_task SET status=$2, reviewer_id=$3, reason=$4, reviewed_at=now(), updated_at=now() WHERE id=$1`,
			taskID, newStatus, reviewerID, reason)
		return e
	}); err != nil {
		return contract.Verdict{}, err
	}

	if confirmed {
		if task.Source == contract.SourceReport {
			rc := strings.TrimPrefix(task.Reason, "举报:")
			_ = m.markReportHandled(ctx, task.SubjectType, task.SubjectID)
			_ = m.emit(ctx, contract.NewReportConfirmed(ids.New().String(), contract.ReportConfirmedPayload{
				TargetType: task.SubjectType, TargetID: task.SubjectID, ReasonCode: rc,
			}))
		} else {
			_ = m.emit(ctx, contract.NewModerationRejected(ids.New().String(), contract.ModerationPayload{
				SubjectType: task.SubjectType, SubjectID: task.SubjectID, ReasonCode: "review_reject",
			}))
		}
		if v.UserID != 0 {
			total, e := m.addViolation(ctx, task, v.UserID)
			if e == nil {
				v.Points = 1
				v.Total = total
				if total >= 3 {
					_ = m.Restrict(ctx, v.UserID, 7, "违规记分达 3 次，限制发布 7 天")
					v.Restricted = true
				}
			}
		}
	}
	return v, nil
}

// ReviewQueue 待审队列（按 SLA 到期升序）。
func (m *Module) ReviewQueue(ctx context.Context, limit int) ([]contract.ReviewTask, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.deps.Pool.Query(ctx, `
		SELECT id, source, subject_type, subject_id, risk_level, status, (sla_due_at < now()) AS overdue
		FROM trs_review_task WHERE status=0 ORDER BY sla_due_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.ReviewTask
	for rows.Next() {
		var t contract.ReviewTask
		if err := rows.Scan(&t.ID, &t.Source, &t.SubjectType, &t.SubjectID, &t.RiskLevel, &t.Status, &t.SLAOverdue); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ViolationPoints 累计违规分。
func (m *Module) ViolationPoints(ctx context.Context, userID int64) (int, error) {
	var n int
	err := m.deps.Pool.QueryRow(ctx, `SELECT COALESCE(sum(points),0) FROM trs_violation_score WHERE user_id=$1`, userID).Scan(&n)
	return n, err
}

// SLAMetrics 队列指标。
func (m *Module) SLAMetrics(ctx context.Context) (contract.SLAMetric, error) {
	var s contract.SLAMetric
	var avg *float64
	err := m.deps.Pool.QueryRow(ctx, `
		SELECT
			count(1) FILTER (WHERE status=0),
			count(1) FILTER (WHERE status=0 AND sla_due_at < now()),
			avg(EXTRACT(EPOCH FROM (reviewed_at - created_at))) FILTER (WHERE reviewed_at IS NOT NULL)
		FROM trs_review_task`).Scan(&s.Pending, &s.Overdue, &avg)
	if avg != nil {
		s.AvgReviewSeconds = int(*avg)
	}
	return s, err
}

// ---- 内部 ----

func (m *Module) pool() *db.Pool { return m.deps.Pool }

func (m *Module) provider(subjectType string) contract.ReviewSubjectProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.providers[subjectType]
}

func (m *Module) loadTask(ctx context.Context, id int64) (contract.ReviewTask, error) {
	var t contract.ReviewTask
	err := m.deps.Pool.QueryRow(ctx,
		`SELECT id, source, subject_type, subject_id, risk_level, status, COALESCE(reason,'') FROM trs_review_task WHERE id=$1`, id).
		Scan(&t.ID, &t.Source, &t.SubjectType, &t.SubjectID, &t.RiskLevel, &t.Status, &t.Reason)
	if err != nil {
		return t, err
	}
	return t, nil
}

func (m *Module) markReportHandled(ctx context.Context, targetType string, targetID int64) error {
	_, err := m.deps.Pool.Exec(ctx,
		`UPDATE trs_report SET status=1, handled_at=now()
		 WHERE status=0 AND target_ref->>'target_type'=$1 AND (target_ref->>'target_id')::bigint=$2`,
		targetType, targetID)
	return err
}

func (m *Module) addViolation(ctx context.Context, task contract.ReviewTask, userID int64) (int, error) {
	if _, err := m.deps.Pool.Exec(ctx,
		`INSERT INTO trs_violation_score (user_id, source_task_id, points, created_at) VALUES ($1,$2,1,now())`,
		userID, task.ID); err != nil {
		return 0, err
	}
	total, err := m.ViolationPoints(ctx, userID)
	if err != nil {
		return 0, err
	}
	_ = m.emit(ctx, contract.NewViolationRecorded(ids.New().String(), contract.ViolationPayload{
		UserID: userID, Points: 1, Total: total, SourceTaskID: task.ID,
	}))
	return total, nil
}

func (m *Module) createTask(ctx context.Context, source int16, subjectType string, subjectID, submitterID int64, risk int16, reason string) error {
	_, err := m.deps.Pool.Exec(ctx, `
		INSERT INTO trs_review_task (id, source, subject_type, subject_id, submitter_id, risk_level, status, reason, sla_due_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,0,$7,now()+interval '24 hours',now(),now())`,
		int64(ids.New()), source, subjectType, subjectID, submitterID, risk, reason)
	return err
}

func (m *Module) emit(ctx context.Context, e eventbus.Event) error {
	if m.deps.Bus == nil {
		return nil
	}
	return m.deps.Pool.InTx(ctx, func(tx db.Tx) error {
		return m.deps.Bus.Publish(ctx, tx, e)
	})
}

// highRiskCategory 高风险类目关键词（金融/医疗/政府项目等，人工抽检）。
func highRiskCategory(text string) string {
	cats := map[string][]string{
		"finance": {"金融", "贷款", "理财", "投资", "融资"},
		"medical": {"医疗", "药品", "医院", "诊疗"},
		"gov":     {"政府", "政务服务", "事业单位"},
	}
	for cat, words := range cats {
		for _, w := range words {
			if strings.Contains(text, w) {
				return cat
			}
		}
	}
	return ""
}

func parseRef(ref string) (string, int64) {
	i := strings.LastIndex(ref, ":")
	if i <= 0 {
		return ref, 0
	}
	id, _ := strconv.ParseInt(ref[i+1:], 10, 64)
	return ref[:i], id
}

// capProvider 「是否被限制发布」能力提供者。
type capProvider struct{ m *Module }

func (p capProvider) Name() string { return "trust" }

func (p capProvider) Has(ctx context.Context, userID int64, capability string) (bool, error) {
	if capability != "publish.allowed" {
		return false, nil
	}
	restricted, err := p.m.IsRestricted(ctx, userID)
	if err != nil {
		return false, err
	}
	return !restricted, nil
}

// ---- HTTP 层 ----

// trust 错误码（contracts/errors/README.md 02 段）。
const (
	codeParamInvalid = 1     // 000001
	codeUnauthorized = 2     // 000002
	codeNotFound     = 4     // 000004
	codeInternal     = 9     // 000009
	codeRestricted   = 20003 // 020003 用户处于限制期
	codeBadReason    = 20004 // 020004 举报原因非法
	codeTaskNotFound = 20005 // 020005 审核任务不存在或已处理
)

func (m *Module) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			writeErr(w, codeUnauthorized, "未登录")
			return
		}
		if m.deps.Auth == nil {
			next(w, req.WithContext(context.WithValue(req.Context(), ctxUserID{}, int64(0))))
			return
		}
		uid, err := m.deps.Auth(req.Context(), token)
		if err != nil {
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
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func (m *Module) handleReasons(w http.ResponseWriter, _ *http.Request) {
	writeData(w, map[string]any{"items": contract.ReportReasons})
}

func (m *Module) handleReport(w http.ResponseWriter, req *http.Request) {
	var body struct {
		TargetRef  string `json:"target_ref"`
		ReasonCode string `json:"reason_code"`
		Detail     string `json:"detail"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.TargetRef == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.Report(req.Context(), userIDFrom(req.Context()), body.TargetRef, body.ReasonCode, body.Detail); err != nil {
		if !contract.ValidReportReason(body.ReasonCode) {
			writeErr(w, codeBadReason, "举报原因非法")
			return
		}
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"reported": true})
}

func (m *Module) handleQueue(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	tasks, err := m.ReviewQueue(req.Context(), limit)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"items": tasks})
}

func (m *Module) handleResolve(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	var body struct {
		Confirmed bool   `json:"confirmed"`
		Reason    string `json:"reason"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	v, err := m.ResolveReview(req.Context(), id, userIDFrom(req.Context()), body.Confirmed, body.Reason)
	if err != nil {
		if strings.Contains(err.Error(), "已处理") {
			writeErr(w, codeTaskNotFound, err.Error())
			return
		}
		writeErr(w, codeNotFound, "审核任务不存在或已处理")
		return
	}
	m.auditAdmin(req, v)
	writeData(w, v)
}

// auditAdmin 复核结论审计。
func (m *Module) auditAdmin(req *http.Request, v contract.Verdict) {
	if m.deps.Audit == nil {
		return
	}
	ip := req.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin",
		Action:    "trust:review:resolve",
		TargetRef: fmt.Sprintf(`{"type":"%s","id":"%d"}`, v.SubjectType, v.SubjectID),
		Detail:    map[string]any{"points": v.Points, "restricted": v.Restricted},
		Result:    audit.ResultOK, IP: ip, UserAgent: req.UserAgent(),
	})
}

func (m *Module) handleMetrics(w http.ResponseWriter, req *http.Request) {
	s, err := m.SLAMetrics(req.Context())
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, s)
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeNotFound, codeTaskNotFound:
		return http.StatusNotFound
	case codeParamInvalid, codeBadReason:
		return http.StatusBadRequest
	case codeRestricted:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func writeErr(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus(code))
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}

var _ contract.Facade = (*Module)(nil)
