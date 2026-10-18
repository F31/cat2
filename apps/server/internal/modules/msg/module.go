// Package msg R1 消息模块：单聊会话、消息、增量同步、未读、分区留存；群聊成员由扩展点解析。
package msg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/obs"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/settings"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/modules/msg/contract"
	"catcube/internal/modules/msg/internal/app"
	"catcube/internal/modules/msg/internal/domain"
	"catcube/internal/modules/msg/internal/infra"
	trustcontract "catcube/internal/modules/trust/contract"
	"catcube/internal/ports"
	"catcube/internal/shared/entityref"
)

// Deps 装配依赖。
type Deps struct {
	Pool           *db.Pool
	Bus            *eventbus.Bus
	Identity       idcontract.Facade
	Policy         *policy.Engine
	Resolver       *entityref.Registry
	Realtime       ports.Realtime // 一期 Noop（纯异步，无 WebSocket）
	Audit          *audit.Recorder
	Storage        ports.Storage                  // 归档压缩上传（未配置对象存储时开发态本地回退）
	MemberResolver contract.SessionMemberResolver // 群成员解析（cube 注册）
	Settings       *settings.Registry             // 留存天数等业务配置（p0 #2/D8）
}

// msgSchemas 业务配置（超管可配，热生效）。
var msgSchemas = []settings.Schema{
	{Key: "msg.retention.days", Type: settings.TypeInt, Default: "180",
		Description: "聊天留存天数（默认 180=6 个月；p0 #2/D8）"},
}

// Module msg 模块。
type Module struct {
	deps Deps
	svc  *app.Service
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "msg" }
func (m *Module) Rank() module.Rank   { return module.RankR1 }
func (m *Module) DependsOn() []string { return []string{"identity"} }

func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{
		Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus,
		Policy: m.deps.Policy, Resolver: m.deps.Resolver, Storage: m.deps.Storage,
		MemberResolver: m.deps.MemberResolver,
		RetentionDays: func(ctx context.Context) int {
			if m.deps.Settings == nil {
				return 180
			}
			if v, err := m.deps.Settings.Int(ctx, 1, "msg.retention.days"); err == nil && v > 0 {
				return int(v)
			}
			return 180
		},
	})
	r.Settings = append(r.Settings, msgSchemas...)
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/sessions", Handler: m.auth(m.handleEnsure)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/sessions", Handler: m.auth(m.handleList)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/sessions/{id}/messages", Handler: m.auth(m.handleSend)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/sessions/{id}/messages", Handler: m.auth(m.handleSync)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/sessions/{id}/read", Handler: m.auth(m.handleMarkRead)},
	)
	r.Jobs = append(r.Jobs,
		module.JobDef{Name: "msg.ensure_partition", Interval: 24 * time.Hour, Run: m.svc.EnsurePartitions},
		module.JobDef{Name: "msg.archive", Interval: 24 * time.Hour, Run: m.svc.Archive},
	)
	// 立方成员变更 → 群成员同步（订阅 cube 事件，topic 字面量）
	for _, topic := range []string{"cube.MemberJoined.v1", "cube.MemberRemoved.v1", "cube.CubeDissolved.v1", "cube.CubeCreated.v1"} {
		r.EventSubs = append(r.EventSubs, eventbus.Subscription{
			Topic: topic, Handler: func(ctx context.Context, _ eventbus.Event) error { return m.svc.SyncGroups(ctx) },
		})
	}
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

// Archive 手动触发归档（运维入口；定时由 msg.archive Job 调用同一实现）。
func (m *Module) Archive(ctx context.Context) error { return m.svc.Archive(ctx) }

func (m *Module) EnsureSession(ctx context.Context, userID int64, targetRef string) (contract.EnsureResult, error) {
	return m.svc.EnsureSession(ctx, userID, targetRef)
}
func (m *Module) Sessions(ctx context.Context, userID int64) ([]contract.Session, error) {
	return m.svc.Sessions(ctx, userID)
}
func (m *Module) Send(ctx context.Context, userID, sessionID int64, msgType int16, content string) (contract.Message, error) {
	return m.svc.Send(ctx, userID, sessionID, msgType, content)
}
func (m *Module) EnsureGroup(ctx context.Context, bizRef, title string, creatorID int64) (contract.Session, error) {
	return m.svc.EnsureGroup(ctx, bizRef, title, creatorID)
}
func (m *Module) SyncGroups(ctx context.Context) error { return m.svc.SyncGroups(ctx) }

// AdminHistory 后台合规查看群聊记录。
func (m *Module) AdminHistory(ctx context.Context, bizRefs []string, limit int) ([]contract.AdminChatSession, error) {
	return m.svc.AdminHistory(ctx, bizRefs, limit)
}

// SetMemberResolver 注入群成员解析器（cube 注册；msg 不认识猫立方）。
func (m *Module) SetMemberResolver(r contract.SessionMemberResolver) { m.deps.MemberResolver = r }

func (m *Module) Sync(ctx context.Context, userID, sessionID, cursor int64, limit int) (contract.SyncResult, error) {
	return m.svc.Sync(ctx, userID, sessionID, cursor, limit)
}
func (m *Module) MarkRead(ctx context.Context, userID, sessionID, readSeq int64) error {
	return m.svc.MarkRead(ctx, userID, sessionID, readSeq)
}

// ---- 审核对象提供者（trust 扩展点：聊天内容复核）----

// ProviderFor 返回聊天消息的 trust.ReviewSubjectProvider（wire 注册进 trust）。
func (m *Module) ProviderFor() trustcontract.ReviewSubjectProvider { return reviewProvider{m} }

type reviewProvider struct{ m *Module }

func (p reviewProvider) SubjectType() string { return "msg.message" }

func (p reviewProvider) Extract(ctx context.Context, subjectID int64) (string, []string, error) {
	var content string
	err := p.m.deps.Pool.QueryRow(ctx, `SELECT content FROM msg_message WHERE id=$1`, subjectID).Scan(&content)
	return content, nil, err
}

func (p reviewProvider) OwnerOf(ctx context.Context, subjectID int64) (int64, error) {
	var sender int64
	err := p.m.deps.Pool.QueryRow(ctx, `SELECT sender_id FROM msg_message WHERE id=$1`, subjectID).Scan(&sender)
	return sender, err
}

// ApplyVerdict 聊天消息一期不做状态回落（结论用于记分/通知）。
func (p reviewProvider) ApplyVerdict(context.Context, int64, bool, string) error { return nil }

// ---- HTTP 层 ----

// msg 错误码（contracts/errors/README.md 07 段）。
const (
	codeParamInvalid    = 1     // 000001
	codeUnauthorized    = 2     // 000002
	codeInternal        = 9     // 000009
	codeBadTarget       = 70001 // 070001 目标引用非法
	codeNotParticipant  = 70002 // 070002 非会话成员
	codeSessionNotFound = 70003 // 070003 会话不存在
	codeMemberRequired  = 70004 // 070004 发起单聊需要会员权益
	codeBadContent      = 70005 // 070005 消息内容非法
	codeSelfSession     = 70006 // 070006 不能与自己发起会话
)

func (m *Module) handleEnsure(w http.ResponseWriter, req *http.Request) {
	var body struct {
		TargetRef string `json:"target_ref"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.TargetRef == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	res, err := m.svc.EnsureSession(req.Context(), userIDFrom(req.Context()), body.TargetRef)
	if err != nil {
		writeMsgErr(w, err)
		return
	}
	obs.Default().Inc("catcube_session_ensure_total")
	writeData(w, res)
}

func (m *Module) handleList(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.Sessions(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeMsgErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleSend(w http.ResponseWriter, req *http.Request) {
	sid, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	var body struct {
		MsgType int16  `json:"msg_type"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if body.MsgType == 0 {
		body.MsgType = contract.MsgText
	}
	msg, err := m.svc.Send(req.Context(), userIDFrom(req.Context()), sid, body.MsgType, body.Content)
	if err != nil {
		if errors.Is(err, domain.ErrNotParticipant) {
			m.auditDenied(req, "msg:message:send", sid, userIDFrom(req.Context()))
		}
		writeMsgErr(w, err)
		return
	}
	writeData(w, msg)
}

func (m *Module) handleSync(w http.ResponseWriter, req *http.Request) {
	sid, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	cursor, _ := strconv.ParseInt(req.URL.Query().Get("cursor"), 10, 64)
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	res, err := m.svc.Sync(req.Context(), userIDFrom(req.Context()), sid, cursor, limit)
	if err != nil {
		if errors.Is(err, domain.ErrNotParticipant) {
			m.auditDenied(req, "msg:message:sync", sid, userIDFrom(req.Context()))
		}
		writeMsgErr(w, err)
		return
	}
	writeData(w, res)
}

func (m *Module) handleMarkRead(w http.ResponseWriter, req *http.Request) {
	sid, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	var body struct {
		ReadSeq int64 `json:"read_seq"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.MarkRead(req.Context(), userIDFrom(req.Context()), sid, body.ReadSeq); err != nil {
		if errors.Is(err, domain.ErrNotParticipant) {
			m.auditDenied(req, "msg:message:markread", sid, userIDFrom(req.Context()))
		}
		writeMsgErr(w, err)
		return
	}
	writeData(w, map[string]any{"read": true})
}

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

// auditDenied 记录权限拒绝审计（非会话成员拉取/发送），安全审计最有价值的部分。
func (m *Module) auditDenied(req *http.Request, action string, sessionID, userID int64) {
	if m.deps.Audit == nil {
		return
	}
	ip := req.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: userID, ActorType: audit.ActorUser, Channel: "mp", Action: action,
		TargetRef: fmt.Sprintf(`{"type":"msg.session","id":"%d"}`, sessionID),
		Result:    audit.ResultDenied, IP: ip, UserAgent: req.UserAgent(),
	})
}

func writeMsgErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidTarget):
		writeErr(w, codeBadTarget, err.Error())
	case errors.Is(err, domain.ErrSelfSession):
		writeErr(w, codeSelfSession, err.Error())
	case errors.Is(err, domain.ErrMemberRequired):
		writeErr(w, codeMemberRequired, err.Error())
	case errors.Is(err, domain.ErrSessionNotFound):
		writeErr(w, codeSessionNotFound, err.Error())
	case errors.Is(err, domain.ErrNotParticipant):
		writeErr(w, codeNotParticipant, err.Error())
	case errors.Is(err, domain.ErrBadMsgType), errors.Is(err, domain.ErrEmptyContent), errors.Is(err, domain.ErrContentTooLong):
		writeErr(w, codeBadContent, err.Error())
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeMemberRequired, codeNotParticipant:
		return http.StatusForbidden
	case codeSessionNotFound:
		return http.StatusNotFound
	case codeParamInvalid, codeBadTarget, codeBadContent, codeSelfSession:
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

var (
	_ contract.Facade                     = (*Module)(nil)
	_ trustcontract.ReviewSubjectProvider = (*reviewProvider)(nil)
)
