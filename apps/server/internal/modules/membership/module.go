// Package membership R3 会员模块：等级与权益、报价、开通/续期（trade 履约）、到期回落、子账号。
package membership

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/settings"
	idcontract "catcube/internal/modules/identity/contract"
	memcontract "catcube/internal/modules/membership/contract"
	"catcube/internal/modules/membership/internal/app"
	"catcube/internal/modules/membership/internal/domain"
	"catcube/internal/modules/membership/internal/infra"
	tradecontract "catcube/internal/modules/trade/contract"
	verifycontract "catcube/internal/modules/verify/contract"
)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Trade    tradecontract.Facade
	Verify   verifycontract.Facade
	Identity idcontract.Facade
	Settings *settings.Registry
	Policy   *policy.Engine
}

// Module membership 模块。
type Module struct {
	deps Deps
	svc  *app.Service
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "membership" }
func (m *Module) Rank() module.Rank   { return module.RankR3 }
func (m *Module) DependsOn() []string { return []string{"trade", "verify", "identity"} }

func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{
		Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus,
		Trade: m.deps.Trade, Verify: m.deps.Verify, Identity: m.deps.Identity,
		Settings: m.deps.Settings, Policy: m.deps.Policy,
	})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/membership/quote", Handler: m.handleQuote},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/membership/orders", Handler: m.auth(m.handleOpen)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/membership/me", Handler: m.auth(m.handleMe)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/membership/sub-accounts", Handler: m.auth(m.handleSubs)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/membership/sub-accounts", Handler: m.auth(m.handleInvite)},
		module.Route{Method: http.MethodDelete, Path: "/api/mp/v1/membership/sub-accounts/{subUserID}", Handler: m.auth(m.handleRemove)},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/members", Handler: m.auth(m.handleAdminMembers), Admin: true},
	)
	r.Settings = append(r.Settings, app.Schemas...)
	r.StateMachines = append(r.StateMachines, domain.Machine)
	r.CapProviders = append(r.CapProviders, capProvider{m})
	r.Jobs = append(r.Jobs, module.JobDef{Name: "membership.expire", Interval: 30 * time.Minute, Run: m.svc.ExpireDue})
	return nil
}

// capProvider 能力提供者：会员生效中 / 企业版生效中。
type capProvider struct{ m *Module }

func (p capProvider) Name() string { return "membership" }

func (p capProvider) Has(ctx context.Context, userID int64, capability string) (bool, error) {
	switch capability {
	case "member.active":
		_, active, err := p.m.svc.LevelOf(ctx, userID)
		return active, err
	case "member.enterprise":
		level, active, err := p.m.svc.LevelOf(ctx, userID)
		return active && level == memcontract.LevelEnterprise, err
	default:
		return false, nil
	}
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() memcontract.Facade { return m }

// ExpireDue 手动触发到期回落（运维入口；定时由 membership.expire Job 调用同一实现）。
func (m *Module) ExpireDue(ctx context.Context) error { return m.svc.ExpireDue(ctx) }

func (m *Module) Quote(ctx context.Context, level int16, years int) (memcontract.Quote, error) {
	return m.svc.Quote(ctx, level, years)
}
func (m *Module) Open(ctx context.Context, userID int64, level int16, years int) (memcontract.OrderResult, error) {
	return m.svc.Open(ctx, userID, level, years)
}
func (m *Module) Me(ctx context.Context, userID int64) (memcontract.Member, bool, error) {
	return m.svc.Me(ctx, userID)
}
func (m *Module) LevelOf(ctx context.Context, userID int64) (int16, bool, error) {
	return m.svc.LevelOf(ctx, userID)
}
func (m *Module) SubAccounts(ctx context.Context, masterID int64) ([]memcontract.SubAccount, error) {
	return m.svc.SubAccounts(ctx, masterID)
}
func (m *Module) InviteSub(ctx context.Context, masterID, subUserID int64) error {
	return m.svc.InviteSub(ctx, masterID, subUserID)
}
func (m *Module) RemoveSub(ctx context.Context, masterID, subUserID int64) error {
	return m.svc.RemoveSub(ctx, masterID, subUserID)
}
func (m *Module) RefundQuote(ctx context.Context, orderID int64) (int64, error) {
	return m.svc.RefundQuote(ctx, orderID)
}

// ---- trade.OrderFulfiller 实现 ----

func (m *Module) BizType() string { return "member" }
func (m *Module) Fulfill(ctx context.Context, order tradecontract.Order) error {
	return m.svc.Fulfill(ctx, order)
}

// ---- HTTP 层 ----

// membership 错误码（contracts/errors/README.md 12 段）。
const (
	codeParamInvalid   = 1      // 000001
	codeUnauthorized   = 2      // 000002
	codeConflict       = 5      // 000005
	codeInternal       = 9      // 000009
	codeLevelUnknown   = 120001 // 120001
	codeYearsInvalid   = 120002 // 120002
	codeEnterpriseCert = 120003 // 120003
	codeNotEnterprise  = 120004 // 120004
	codeSubLimit       = 120005 // 120005
	codeSubNotRealname = 120006 // 120006
)

func (m *Module) handleQuote(w http.ResponseWriter, req *http.Request) {
	level, _ := strconv.Atoi(req.URL.Query().Get("level"))
	years, _ := strconv.Atoi(req.URL.Query().Get("years"))
	if years == 0 {
		years = 1
	}
	q, err := m.svc.Quote(req.Context(), int16(level), years)
	if err != nil {
		writeMemErr(w, err)
		return
	}
	writeData(w, q)
}

func (m *Module) handleOpen(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Level int16 `json:"level"`
		Years int   `json:"years"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if body.Years == 0 {
		body.Years = 1
	}
	res, err := m.svc.Open(req.Context(), userIDFrom(req.Context()), body.Level, body.Years)
	if err != nil {
		writeMemErr(w, err)
		return
	}
	writeData(w, res)
}

func (m *Module) handleMe(w http.ResponseWriter, req *http.Request) {
	mem, active, err := m.svc.Me(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeMemErr(w, err)
		return
	}
	writeData(w, map[string]any{"member": mem, "active": active})
}

func (m *Module) handleSubs(w http.ResponseWriter, req *http.Request) {
	subs, err := m.svc.SubAccounts(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeMemErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": subs})
}

func (m *Module) handleInvite(w http.ResponseWriter, req *http.Request) {
	var body struct {
		SubUserID int64 `json:"sub_user_id,string"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.SubUserID == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.InviteSub(req.Context(), userIDFrom(req.Context()), body.SubUserID); err != nil {
		writeMemErr(w, err)
		return
	}
	writeData(w, map[string]any{"invited": true})
}

func (m *Module) handleRemove(w http.ResponseWriter, req *http.Request) {
	subID, err := strconv.ParseInt(req.PathValue("subUserID"), 10, 64)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.RemoveSub(req.Context(), userIDFrom(req.Context()), subID); err != nil {
		writeMemErr(w, err)
		return
	}
	writeData(w, map[string]any{"removed": true})
}

// ---- 后台管理（RBAC 细化在 p4）----

func (m *Module) handleAdminMembers(w http.ResponseWriter, req *http.Request) {
	level := int16(-1)
	status := int16(-1)
	if s := req.URL.Query().Get("level"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			level = int16(v)
		}
	}
	if s := req.URL.Query().Get("status"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			status = int16(v)
		}
	}
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := m.svc.AdminList(req.Context(), level, status, limit)
	if err != nil {
		writeMemErr(w, err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, map[string]any{
			"user_id":       strconv.FormatInt(r.UserID, 10),
			"level":         r.Level,
			"status":        r.Status,
			"started_at":    r.StartedAt,
			"expires_at":    r.ExpiresAt,
			"last_order_id": strconv.FormatInt(r.LastOrderID, 10),
		})
	}
	writeData(w, map[string]any{"items": items})
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
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func writeMemErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrLevelUnknown):
		writeErr(w, codeLevelUnknown, err.Error())
	case errors.Is(err, domain.ErrYearsInvalid):
		writeErr(w, codeYearsInvalid, err.Error())
	case errors.Is(err, domain.ErrEnterpriseCert):
		writeErr(w, codeEnterpriseCert, err.Error())
	case errors.Is(err, domain.ErrNotEnterprise):
		writeErr(w, codeNotEnterprise, err.Error())
	case errors.Is(err, domain.ErrSubLimit):
		writeErr(w, codeSubLimit, err.Error())
	case errors.Is(err, domain.ErrSubNotRealname):
		writeErr(w, codeSubNotRealname, err.Error())
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeEnterpriseCert, codeNotEnterprise:
		return http.StatusForbidden
	case codeSubLimit, codeConflict:
		return http.StatusConflict
	case codeParamInvalid, codeLevelUnknown, codeYearsInvalid, codeSubNotRealname:
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
	_ memcontract.Facade           = (*Module)(nil)
	_ tradecontract.OrderFulfiller = (*Module)(nil)
)
