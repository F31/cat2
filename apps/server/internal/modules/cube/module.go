// Package cube R3 猫立方模块：独立实体空间、邀请制成员、协作实体（研究/承接/任务/报备）、
// 订阅三档与分摊成团、立方内项目引用。注册 OrderFulfiller/SessionMemberResolver/能力与实体解析。
package cube

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
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/kernel/settings"
	contentcontract "catcube/internal/modules/content/contract"
	"catcube/internal/modules/cube/contract"
	"catcube/internal/modules/cube/internal/app"
	"catcube/internal/modules/cube/internal/domain"
	"catcube/internal/modules/cube/internal/infra"
	idcontract "catcube/internal/modules/identity/contract"
	msgcontract "catcube/internal/modules/msg/contract"
	tradecontract "catcube/internal/modules/trade/contract"
	"catcube/internal/shared/entityref"
)

// Deps 装配依赖。
type Deps struct {
	Pool      *db.Pool
	Bus       *eventbus.Bus
	Identity  idcontract.Facade
	Policy    *policy.Engine
	Entities  *entityref.Registry
	Audit     *audit.Recorder
	Encryptor *sensitive.Encryptor
	Trade     tradecontract.Facade
	Settings  *settings.Registry
	Content   contentcontract.Facade
	Msg       msgcontract.Facade
}

// Module cube 模块。
type Module struct {
	deps Deps
	svc  *app.Service
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "cube" }
func (m *Module) Rank() module.Rank   { return module.RankR3 }
func (m *Module) DependsOn() []string { return []string{"identity", "content", "trade", "msg"} }

// cube 错误码（contracts/errors/README.md 13 段）。
const (
	codeParamInvalid = 1 // 000001
	codeUnauthorized = 2 // 000002
	codeInternal     = 9 // 000009
	codeNotFound     = 130001
	codeNoPermission = 130002
	codeNotRealname  = 130003
	codeHoney        = 130004
	codeInviteBad    = 130005
	codeRemoved      = 130006
	codeCubeLimit    = 130007
	codeMemberLimit  = 130008
	codeInvalidInput = 130009
	codeTaskTrans    = 130010
	codeRejectReason = 130011
	codeBadTier      = 130013
	codeApportionBad = 130014
)

func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{
		Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus,
		Policy: m.deps.Policy, Identity: m.deps.Identity, Encryptor: m.deps.Encryptor,
		Trade: m.deps.Trade, Settings: m.deps.Settings, Content: m.deps.Content, Entities: m.deps.Entities,
		Msg: m.deps.Msg,
	})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes", Handler: m.auth(m.handleCreate)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cubes", Handler: m.auth(m.handleListMine)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/join", Handler: m.auth(m.handleJoin)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cubes/{id}", Handler: m.auth(m.handleDetail)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/invites", Handler: m.auth(m.handleInvite)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cubes/{id}/members", Handler: m.auth(m.handleMembers)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/members/{uid}/remove", Handler: m.auth(m.handleRemove)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/leave", Handler: m.auth(m.handleLeave)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/transfer", Handler: m.auth(m.handleTransfer)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/dissolve", Handler: m.auth(m.handleDissolve)},
		// 协作实体（§2）
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/projects", Handler: m.auth(m.handleCreateProject)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cubes/{id}/projects", Handler: m.auth(m.handleListProjects)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/projects/{pid}/close", Handler: m.auth(m.handleCloseProject)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/projects/{pid}/reopen", Handler: m.auth(m.handleReopenProject)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cubes/{id}/projects/{pid}/research", Handler: m.auth(m.handleListResearch)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/projects/{pid}/research", Handler: m.auth(m.handleAddResearch)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/tasks", Handler: m.auth(m.handleCreateTask)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cubes/{id}/tasks", Handler: m.auth(m.handleListTasks)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/tasks/{tid}/start", Handler: m.auth(m.taskAction("start"))},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/tasks/{tid}/submit", Handler: m.auth(m.taskAction("submit"))},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/tasks/{tid}/confirm", Handler: m.auth(m.taskAction("confirm"))},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/tasks/{tid}/reopen", Handler: m.auth(m.taskAction("reopen"))},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/reports", Handler: m.auth(m.handleSubmitReport)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cubes/{id}/reports", Handler: m.auth(m.handleListReports)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/reports/{rid}/confirm", Handler: m.auth(m.handleConfirmReport)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/reports/{rid}/reject", Handler: m.auth(m.handleRejectReport)},
		// 项目关联（§4）
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/link-requests", Handler: m.auth(m.handleRequestLink)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cubes/{id}/link-requests", Handler: m.auth(m.handleLinkRequests)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/link-requests/{reqID}/decide", Handler: m.auth(m.handleDecideLink)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/projects/{pid}/publish", Handler: m.auth(m.handlePublishToPlatform)},
		// 订阅与分摊（§3）
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cubes/plans", Handler: m.handlePlans},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/plan", Handler: m.auth(m.handleChangePlan)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/apportion", Handler: m.auth(m.handleInitiateApportion)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/apportion/pledge", Handler: m.auth(m.handlePledgeApportion)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cubes/{id}/apportion/exit", Handler: m.auth(m.handleExitApportion)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cube-apportions/{groupID}", Handler: m.auth(m.handleApportionDetail)},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/cubes", Handler: m.auth(m.handleAdminList), Admin: true},
	)
	r.StateMachines = append(r.StateMachines, domain.MachineCube, domain.MachineMember,
		domain.MachineProject, domain.MachineTask, domain.MachineReport, domain.MachinePlan)
	r.Settings = append(r.Settings, app.Schemas...)
	// 订阅 trade.ApportionFormed：成团开通订阅 + 共建成员入立方
	r.EventSubs = append(r.EventSubs, eventbus.Subscription{
		Topic: "trade.ApportionFormed.v1", Handler: func(ctx context.Context, e eventbus.Event) error {
			return m.svc.HandleApportionFormed(ctx, e.Payload)
		},
	})
	r.AuditPoints = append(r.AuditPoints,
		audit.Point{Action: "cube:dissolve", Path: "/api/mp/v1/cubes/{id}/dissolve", Required: true, LogFailed: true, Module: "cube"},
		audit.Point{Action: "cube:transfer", Path: "/api/mp/v1/cubes/{id}/transfer", Required: true, LogFailed: true, Module: "cube"},
	)
	r.CapProviders = append(r.CapProviders, capProvider{m})
	r.Jobs = append(r.Jobs, module.JobDef{Name: "cube.plan_expire", Interval: 30 * time.Minute, Run: m.svc.ExpireDue})
	if m.deps.Entities != nil {
		m.deps.Entities.Register("cube", m)
	}
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

// ---- contract.Facade ----

func (m *Module) Brief(ctx context.Context, cubeID int64) (contract.CubeBrief, error) {
	return m.svc.Brief(ctx, cubeID)
}
func (m *Module) RoleOf(ctx context.Context, cubeID, userID int64) (int16, error) {
	return m.svc.RoleOf(ctx, cubeID, userID)
}
func (m *Module) ListMine(ctx context.Context, userID int64) ([]contract.CubeBrief, error) {
	return m.svc.ListMine(ctx, userID)
}
func (m *Module) Detail(ctx context.Context, viewerID, cubeID int64) (contract.CubeDetail, error) {
	return m.svc.Detail(ctx, viewerID, cubeID)
}
func (m *Module) Create(ctx context.Context, userID int64, in contract.CreateInput) (contract.CubeBrief, error) {
	return m.svc.Create(ctx, userID, in)
}
func (m *Module) Invite(ctx context.Context, cubeID, byUserID int64) (contract.InviteTicket, error) {
	return m.svc.Invite(ctx, cubeID, byUserID)
}
func (m *Module) JoinByCode(ctx context.Context, userID int64, code string) (contract.CubeBrief, error) {
	return m.svc.JoinByCode(ctx, userID, code)
}
func (m *Module) Members(ctx context.Context, cubeID int64) ([]contract.Member, error) {
	return m.svc.Members(ctx, cubeID)
}
func (m *Module) Transfer(ctx context.Context, cubeID, from, to int64) error {
	return m.svc.Transfer(ctx, cubeID, from, to)
}
func (m *Module) Dissolve(ctx context.Context, cubeID, userID int64) error {
	return m.svc.Dissolve(ctx, cubeID, userID)
}
func (m *Module) RemoveMember(ctx context.Context, cubeID, operatorID, targetID int64, reason string) error {
	return m.svc.RemoveMember(ctx, cubeID, operatorID, targetID, reason)
}
func (m *Module) Leave(ctx context.Context, cubeID, userID int64) error {
	return m.svc.Leave(ctx, cubeID, userID)
}
func (m *Module) HasActiveMembership(ctx context.Context, userID int64) (bool, error) {
	return m.svc.HasActiveMembership(ctx, userID)
}

// SessionMemberResolver 群成员解析器（cube 注册给 msg；成员=立方成员表）。
func (m *Module) SessionMemberResolver() msgcontract.SessionMemberResolver {
	return memberResolver{m: m}
}

type memberResolver struct{ m *Module }

func (r memberResolver) Members(ctx context.Context, sessionRef string) ([]int64, error) {
	cubeID := int64(0)
	switch {
	case strings.HasPrefix(sessionRef, "cube.project:"):
		pid, err := strconv.ParseInt(strings.TrimPrefix(sessionRef, "cube.project:"), 10, 64)
		if err != nil {
			return nil, nil
		}
		id, err := r.m.svc.ProjectCube(ctx, pid)
		if err != nil {
			return nil, nil
		}
		cubeID = id
	case strings.HasPrefix(sessionRef, "cube:"):
		id, err := strconv.ParseInt(strings.TrimPrefix(sessionRef, "cube:"), 10, 64)
		if err != nil {
			return nil, nil
		}
		cubeID = id
	default:
		return nil, nil
	}
	ms, err := r.m.svc.Members(ctx, cubeID)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(ms))
	for _, mm := range ms {
		if mm.Status != contract.MemberRemoved {
			out = append(out, mm.UserID)
		}
	}
	return out, nil
}

// Resolve 实现 entityref.Resolver（cube 展示）。
func (m *Module) Resolve(ctx context.Context, ref entityref.Ref) (entityref.Info, error) {
	if ref.Type != "cube" {
		return entityref.Info{}, errors.New("cube: 不支持的类型")
	}
	b, err := m.svc.Brief(ctx, ref.ID)
	if err != nil {
		return entityref.Info{}, err
	}
	status := "active"
	if b.Status != contract.CubeActive {
		status = "inactive"
	}
	return entityref.Info{Title: b.Name, Status: status, Visibility: "platform"}, nil
}

// ---- HTTP ----

func (m *Module) handleCreate(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Name          string   `json:"name"`
		OrgID         int64    `json:"org_id,string"`
		RegionCode    string   `json:"region_code"`
		IndustryCodes []string `json:"industry_codes"`
		ChainStage    string   `json:"chain_stage"`
		Direction     string   `json:"direction"`
		Resources     string   `json:"resources"`
		Strength      string   `json:"strength"`
		Cooperation   string   `json:"cooperation"`
		HoneyUserID   int64    `json:"honey_user_id,string"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	res, err := m.svc.Create(req.Context(), userIDFrom(req.Context()), contract.CreateInput{
		Name: body.Name, OrgID: body.OrgID, RegionCode: body.RegionCode, IndustryCodes: body.IndustryCodes,
		ChainStage: body.ChainStage, Direction: body.Direction, Resources: body.Resources,
		Strength: body.Strength, Cooperation: body.Cooperation, HoneyUserID: body.HoneyUserID,
	})
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, res)
}

func (m *Module) handleListMine(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.ListMine(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleJoin(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Code == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	res, err := m.svc.JoinByCode(req.Context(), userIDFrom(req.Context()), body.Code)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, res)
}

func (m *Module) handleDetail(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req, "id")
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	d, err := m.svc.Detail(req.Context(), userIDFrom(req.Context()), id)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, d)
}

func (m *Module) handleInvite(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req, "id")
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	t, err := m.svc.Invite(req.Context(), id, userIDFrom(req.Context()))
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, t)
}

func (m *Module) handleMembers(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req, "id")
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	items, err := m.svc.Members(req.Context(), id)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleRemove(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req, "id")
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	uid, err := pathID(req, "uid")
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	if err := m.svc.RemoveMember(req.Context(), id, userIDFrom(req.Context()), uid, body.Reason); err != nil {
		writeCubeErr(w, err)
		return
	}
	m.auditCube(req, "cube:member:remove", fmt.Sprintf(`{"type":"cube","id":"%d"}`, id),
		map[string]any{"target_user_id": strconv.FormatInt(uid, 10), "reason": body.Reason})
	writeData(w, map[string]any{"removed": true})
}

func (m *Module) handleLeave(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req, "id")
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.Leave(req.Context(), id, userIDFrom(req.Context())); err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"left": true})
}

func (m *Module) handleTransfer(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req, "id")
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	var body struct {
		ToUserID int64 `json:"to_user_id,string"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.ToUserID == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.Transfer(req.Context(), id, userIDFrom(req.Context()), body.ToUserID); err != nil {
		writeCubeErr(w, err)
		return
	}
	m.auditCube(req, "cube:leader:transfer", fmt.Sprintf(`{"type":"cube","id":"%d"}`, id),
		map[string]any{"to_user_id": strconv.FormatInt(body.ToUserID, 10)})
	writeData(w, map[string]any{"transferred": true})
}

func (m *Module) handleDissolve(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req, "id")
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.Dissolve(req.Context(), id, userIDFrom(req.Context())); err != nil {
		writeCubeErr(w, err)
		return
	}
	m.auditCube(req, "cube:dissolve", fmt.Sprintf(`{"type":"cube","id":"%d"}`, id), nil)
	writeData(w, map[string]any{"dissolved": true})
}

func (m *Module) handleAdminList(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	items, err := m.svc.AdminList(req.Context(), req.URL.Query().Get("q"), limit)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

// ---- 能力 ----

type capProvider struct{ m *Module }

func (p capProvider) Name() string { return "cube" }
func (p capProvider) Has(ctx context.Context, userID int64, capability string) (bool, error) {
	switch capability {
	case "cube.member":
		return p.m.svc.HasActiveMembership(ctx, userID)
	default:
		return false, nil
	}
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
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func pathID(req *http.Request, name string) (int64, error) {
	return strconv.ParseInt(req.PathValue(name), 10, 64)
}

// auditCube 记录敏感操作审计（转让/解散/移除）。
func (m *Module) auditCube(req *http.Request, action, targetRef string, detail map[string]any) {
	if m.deps.Audit == nil {
		return
	}
	ip := req.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: userIDFrom(req.Context()), ActorType: audit.ActorUser, Channel: "mp",
		Action: action, TargetRef: targetRef, Detail: detail, Result: audit.ResultOK,
		IP: ip, UserAgent: req.UserAgent(),
	})
}

func writeCubeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeErr(w, codeNotFound, err.Error())
	case errors.Is(err, domain.ErrNoPermission):
		writeErr(w, codeNoPermission, err.Error())
	case errors.Is(err, domain.ErrNotRealname):
		writeErr(w, codeNotRealname, err.Error())
	case errors.Is(err, domain.ErrHoneyRequired):
		writeErr(w, codeHoney, err.Error())
	case errors.Is(err, domain.ErrInviteInvalid):
		writeErr(w, codeInviteBad, err.Error())
	case errors.Is(err, domain.ErrRemoved):
		writeErr(w, codeRemoved, err.Error())
	case errors.Is(err, domain.ErrCubeLimit):
		writeErr(w, codeCubeLimit, err.Error())
	case errors.Is(err, domain.ErrMemberLimit):
		writeErr(w, codeMemberLimit, err.Error())
	case errors.Is(err, domain.ErrTaskTransition), errors.Is(err, domain.ErrProjectClosed):
		writeErr(w, codeTaskTrans, err.Error())
	case errors.Is(err, domain.ErrRejectReason):
		writeErr(w, codeRejectReason, err.Error())
	case errors.Is(err, domain.ErrApportionClosed):
		writeErr(w, codeApportionBad, err.Error())
	case errors.Is(err, domain.ErrBadTier):
		writeErr(w, codeBadTier, err.Error())
	case errors.Is(err, domain.ErrInvalidInput):
		writeErr(w, codeInvalidInput, err.Error())
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeNoPermission, codeNotRealname, codeRemoved:
		return http.StatusForbidden
	case codeNotFound:
		return http.StatusNotFound
	case codeCubeLimit:
		return http.StatusTooManyRequests
	case codeMemberLimit, codeTaskTrans, codeApportionBad:
		return http.StatusConflict
	case codeParamInvalid, codeHoney, codeInviteBad, codeInvalidInput, codeBadTier, codeRejectReason:
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

var _ contract.Facade = (*Module)(nil)
var _ entityref.Resolver = (*Module)(nil)
var _ tradecontract.OrderFulfiller = (*Module)(nil)
var _ msgcontract.SessionMemberResolver = memberResolver{}
