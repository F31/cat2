// Package verify R2 认证模块：一期落地实名（第三方二要素实时核验、状态机 #1、Badge）。
// 个人/企业/机构认证与身份核验（affiliation）在后续增量补；本阶段注册扩展点开口。
package verify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/sensitive"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/modules/verify/contract"
	"catcube/internal/modules/verify/internal/app"
	"catcube/internal/modules/verify/internal/domain"
	"catcube/internal/modules/verify/internal/infra"
	"catcube/internal/ports"
	"catcube/internal/shared/entityref"
)

// Deps 装配依赖。
type Deps struct {
	Pool      *db.Pool
	Bus       *eventbus.Bus
	Verify    ports.RealnameVerify
	Identity  idcontract.Facade
	Encryptor *sensitive.Encryptor
}

// Module verify 模块。
type Module struct {
	deps  Deps
	svc   *app.Service
	store *infra.Store
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "verify" }
func (m *Module) Rank() module.Rank   { return module.RankR2 }
func (m *Module) DependsOn() []string { return []string{"identity"} }

// Register 注册路由与能力提供者。
func (m *Module) Register(r *module.Registry) error {
	m.store = infra.NewStore(m.deps.Pool)
	m.svc = app.New(app.Options{
		Repo: m.store, Pool: m.deps.Pool, Bus: m.deps.Bus, Verify: m.deps.Verify,
		Identity: m.deps.Identity, Encrypt: m.deps.Encryptor,
	})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/certs", Handler: m.auth(m.handleCertify)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/certs/mine", Handler: m.auth(m.handleCertsOf)},
		// 企业/组织审核（个人企业身份由企业管理员审）
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/certs/{id}/review", Handler: m.auth(m.handleOrgReviewCert)},
		// 平台审核认证（全类型）
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/certs/{id}/review", Handler: m.auth(m.handlePlatformReviewCert), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/certs/pending", Handler: m.auth(m.handlePendingCerts), Admin: true},
		// 一人多单位（归属核验）
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/affiliations", Handler: m.auth(m.handleApplyAffiliation)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/affiliations/mine", Handler: m.auth(m.handleMyAffiliations)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/affiliations/pending", Handler: m.auth(m.handlePendingAffiliations)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/affiliations/{id}/verify", Handler: m.auth(m.handleVerifyAffiliation)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/affiliations/{id}/primary", Handler: m.auth(m.handleSetPrimary)},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/affiliations/{id}/resolve", Handler: m.auth(m.handleResolveAffiliation), Admin: true},
		// 认证标识配置（后台：新增类型=加配置，不改接口）
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/badges", Handler: m.auth(m.handleBadgeDefs), Admin: true},
		module.Route{Method: http.MethodPut, Path: "/api/admin/v1/badges/{code}", Handler: m.auth(m.handleUpsertBadge), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/badges/grant", Handler: m.auth(m.handleGrantBadge), Admin: true},
		// 组织管理员机制
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/orgs/{id}/admins", Handler: m.auth(m.handleInviteOrgAdmin)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/orgs/{id}/admins", Handler: m.auth(m.handleOrgAdmins)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/org-admin-invites/{id}/confirm", Handler: m.auth(m.handleConfirmOrgAdmin)},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/orgs/{id}/admins/reset", Handler: m.auth(m.handleResetOrgAdmin), Admin: true},
	)
	r.AuditPoints = append(r.AuditPoints,
		audit.Point{Action: "verify:cert:review", Path: "/api/admin/v1/certs/{id}/review", Required: true, LogFailed: true, Module: "verify"},
		audit.Point{Action: "verify:affiliation:resolve", Path: "/api/admin/v1/affiliations/{id}/resolve", Required: true, LogFailed: true, Module: "verify"},
		audit.Point{Action: "verify:org_admin:reset", Path: "/api/admin/v1/orgs/{id}/admins/reset", Required: true, LogFailed: true, Module: "verify"},
		audit.Point{Action: "verify:badge:config", Path: "/api/admin/v1/badges/{code}", Required: true, LogFailed: true, Module: "verify"},
		audit.Point{Action: "verify:badge:grant", Path: "/api/admin/v1/badges/grant", Required: true, LogFailed: true, Module: "verify"},
	)
	r.StateMachines = append(r.StateMachines, domain.RealnameMachine, domain.MachineCert, domain.MachineAffiliation)
	r.CapProviders = append(r.CapProviders, capProvider{m})
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

func (m *Module) Certify(ctx context.Context, userID int64, in contract.CertifyInput) (contract.Cert, error) {
	return m.svc.Certify(ctx, userID, in)
}
func (m *Module) CertsOf(ctx context.Context, userID int64) ([]contract.Cert, error) {
	return m.svc.CertsOf(ctx, userID)
}
func (m *Module) BadgeDefs(ctx context.Context) ([]contract.BadgeDef, error) {
	defs, err := m.svc.BadgeDefs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.BadgeDef, 0, len(defs))
	for _, d := range defs {
		var style any
		if len(d.Style) > 0 {
			_ = json.Unmarshal(d.Style, &style)
		}
		out = append(out, contract.BadgeDef{Code: d.Code, Label: d.Label, Level: d.Level, Style: style})
	}
	return out, nil
}

func (m *Module) UpsertBadgeDef(ctx context.Context, code, label string, level int16, style []byte) error {
	return m.svc.UpsertBadgeDef(ctx, code, label, level, style)
}

func (m *Module) SetBadgeGrant(ctx context.Context, subjectType string, subjectID int64, code string, grant bool) error {
	return m.svc.SetBadgeGrant(ctx, subjectType, subjectID, code, grant)
}

func (m *Module) ReviewCert(ctx context.Context, certID, reviewerID int64, reviewerType int16, pass bool, reason string) error {
	return m.svc.ReviewCert(ctx, certID, reviewerID, reviewerType, pass, reason)
}
func (m *Module) PendingCerts(ctx context.Context, reviewerType int16, orgID int64, limit int) ([]contract.Cert, error) {
	return m.svc.PendingCerts(ctx, reviewerType, orgID, limit)
}
func (m *Module) ApplyAffiliation(ctx context.Context, userID, orgID int64, role string, proofFileIDs []int64) (contract.Affiliation, error) {
	return m.svc.ApplyAffiliation(ctx, userID, orgID, role, proofFileIDs)
}
func (m *Module) AffiliationsOf(ctx context.Context, userID int64) ([]contract.Affiliation, error) {
	return m.svc.AffiliationsOf(ctx, userID)
}
func (m *Module) PendingAffiliations(ctx context.Context, orgID, reviewerID int64, limit int) ([]contract.Affiliation, error) {
	return m.svc.PendingAffiliations(ctx, orgID, reviewerID, limit)
}
func (m *Module) VerifyAffiliation(ctx context.Context, affID, reviewerID int64, pass bool, reason string) error {
	return m.svc.VerifyAffiliation(ctx, affID, reviewerID, pass, reason)
}
func (m *Module) SetPrimaryAffiliation(ctx context.Context, userID, affID int64) error {
	return m.svc.SetPrimaryAffiliation(ctx, userID, affID)
}
func (m *Module) ResolveAffiliation(ctx context.Context, affID, operatorID int64, pass bool, reason string) error {
	return m.svc.ResolveAffiliation(ctx, affID, operatorID, pass, reason)
}
func (m *Module) InviteOrgAdmin(ctx context.Context, orgID, inviterID, inviteeID int64) error {
	return m.svc.InviteOrgAdmin(ctx, orgID, inviterID, inviteeID)
}
func (m *Module) ConfirmOrgAdmin(ctx context.Context, inviteID, userID int64) error {
	return m.svc.ConfirmOrgAdmin(ctx, inviteID, userID)
}
func (m *Module) OrgAdminsOf(ctx context.Context, orgID int64) ([]contract.OrgAdmin, error) {
	return m.svc.OrgAdminsOf(ctx, orgID)
}
func (m *Module) ResetOrgAdmin(ctx context.Context, orgID, operatorID, userID int64, remove bool) error {
	return m.svc.ResetOrgAdmin(ctx, orgID, operatorID, userID, remove)
}
func (m *Module) IsOrgAdmin(ctx context.Context, orgID, userID int64) (bool, error) {
	return m.svc.IsOrgAdmin(ctx, orgID, userID)
}

func (m *Module) BadgesOf(ctx context.Context, ref entityref.Ref) ([]contract.Badge, error) {
	return m.svc.BadgesOf(ctx, ref)
}
func (m *Module) IsRealname(ctx context.Context, userID int64) (bool, error) {
	return m.svc.IsRealname(ctx, userID)
}

// ---- trust.ReviewSubjectProvider 实现（认证材料；一期实名无材料）----

func (m *Module) SubjectType() string { return m.svc.SubjectType() }
func (m *Module) Extract(ctx context.Context, subjectID int64) (string, []string, error) {
	return m.svc.Extract(ctx, subjectID)
}
func (m *Module) OwnerOf(_ context.Context, subjectID int64) (int64, error) { return subjectID, nil }
func (m *Module) ApplyVerdict(ctx context.Context, subjectID int64, pass bool, reason string) error {
	return m.svc.ApplyVerdict(ctx, subjectID, pass, reason)
}

// ---- HTTP 层 ----

// verify 错误码（contracts/errors/README.md 10 段）。
const (
	codeParamInvalid    = 1      // 000001
	codeUnauthorized    = 2      // 000002
	codeInternal        = 9      // 000009
	codeUpstream        = 8      // 000008
	codeCertInvalid     = 100001 // 100001 姓名/证件号格式或协议校验失败
	codeNotMatched      = 100002 // 100002 实名核验未通过
	codeAlreadyRealname = 100004 // 100004 已完成实名
	codeCertUnsupported = 100005 // 100005 暂不支持的认证类型
	codeNotFoundVerify  = 100006 // 100006 记录不存在或已处理
	codeNoPermVerify    = 100007 // 100007 无审核权限
)

func (m *Module) handleCertify(w http.ResponseWriter, req *http.Request) {
	var in contract.CertifyInput
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	uid := userIDFrom(req.Context())
	if in.CertType != contract.CertRealname {
		cert, err := m.svc.CertifyFor(req.Context(), uid, in)
		if err != nil {
			writeVerifyErr(w, err)
			return
		}
		writeData(w, cert)
		return
	}
	cert, err := m.svc.Certify(req.Context(), uid, in)
	if err != nil {
		writeVerifyErr(w, err)
		return
	}
	writeData(w, cert)
}

func (m *Module) handleCertsOf(w http.ResponseWriter, req *http.Request) {
	certs, err := m.svc.CertsOf(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeVerifyErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": certs})
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
		if err != nil {
			writeErr(w, codeUnauthorized, "登录已失效")
			return
		}
		if brief.AuthzVer != ver {
			writeErr(w, codeUnauthorized, "权限已变更，请重新登录")
			return
		}
		if brief.Status != 0 && brief.Status != 1 {
			writeErr(w, codeUnauthorized, "账号不可用")
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

func writeVerifyErrExt(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrMaterialsRequired), errors.Is(err, app.ErrOrgIDRequired):
		writeErr(w, codeParamInvalid, err.Error())
	case errors.Is(err, app.ErrNoPermission):
		writeErr(w, codeNoPermVerify, err.Error())
	case errors.Is(err, app.ErrCertNotPending), errors.Is(err, app.ErrInviteNotFound), errors.Is(err, app.ErrAffNotFound):
		writeErr(w, codeNotFoundVerify, err.Error())
	case errors.Is(err, app.ErrAffNotVerified):
		writeErr(w, codeCertInvalid, err.Error())
	default:
		logErr("writeVerifyErr default", err)
		writeVerifyErr(w, err)
	}
}

func writeVerifyErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrAgreementRequired), errors.Is(err, domain.ErrInvalidIDCard),
		errors.Is(err, domain.ErrNameRequired):
		writeErr(w, codeCertInvalid, err.Error())
	case errors.Is(err, domain.ErrNotMatched):
		writeErr(w, codeNotMatched, err.Error())
	case errors.Is(err, app.ErrMaterialsRequired), errors.Is(err, app.ErrOrgIDRequired):
		writeErr(w, codeCertInvalid, err.Error())
	case errors.Is(err, app.ErrNoPermission), errors.Is(err, app.ErrOrgAdminExists):
		writeErr(w, codeNoPermVerify, err.Error())
	case errors.Is(err, app.ErrCertNotPending), errors.Is(err, app.ErrInviteNotFound), errors.Is(err, app.ErrAffNotFound):
		writeErr(w, codeNotFoundVerify, err.Error())
	case errors.Is(err, app.ErrAffNotVerified):
		writeErr(w, codeCertInvalid, err.Error())
	case errors.Is(err, domain.ErrAlreadyVerified):
		writeErr(w, codeAlreadyRealname, err.Error())
	case errors.Is(err, domain.ErrUnsupportedCertType):
		writeErr(w, codeCertUnsupported, err.Error())
	case errors.Is(err, domain.ErrUpstream):
		writeErr(w, codeUpstream, "核验服务异常，请稍后重试")
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeParamInvalid, codeCertInvalid, codeNotMatched, codeCertUnsupported:
		return http.StatusBadRequest
	case codeAlreadyRealname:
		return http.StatusConflict
	case codeNotFoundVerify:
		return http.StatusNotFound
	case codeNoPermVerify:
		return http.StatusForbidden
	case codeUpstream:
		return http.StatusBadGateway
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

// capProvider 能力提供者：已实名 / 已认证（一期恒否）/ 组织管理员（一期恒否）。
type capProvider struct{ m *Module }

func (p capProvider) Name() string { return "verify" }

func (p capProvider) Has(ctx context.Context, userID int64, capability string) (bool, error) {
	switch capability {
	case "realname":
		return p.m.svc.IsRealname(ctx, userID)
	case "verified":
		// 企业/机构认证通过的账号（徽标打在 org 主体，一期以申请人账号为准）
		badges, err := p.m.svc.BadgesOf(ctx, entityref.Ref{Type: "user", ID: userID})
		if err != nil {
			return false, err
		}
		for _, b := range badges {
			if b.Code == contract.BadgeVerified || b.Code == contract.BadgeOfficial {
				return true, nil
			}
		}
		return false, nil
	case "org_admin":
		ok, err := p.m.svc.IsOrgAdmin(ctx, userID, userID)
		return ok, err
	default:
		return false, nil
	}
}

var (
	_ contract.Facade = (*Module)(nil)
	_ module.Module   = (*Module)(nil)
)
