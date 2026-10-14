// Package verify R2 认证模块：一期落地实名（第三方二要素实时核验、状态机 #1、Badge）。
// 个人/企业/机构认证与身份核验（affiliation）在后续增量补；本阶段注册扩展点开口。
package verify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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
	)
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
)

func (m *Module) handleCertify(w http.ResponseWriter, req *http.Request) {
	var in contract.CertifyInput
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	cert, err := m.svc.Certify(req.Context(), userIDFrom(req.Context()), in)
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

func writeVerifyErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrAgreementRequired), errors.Is(err, domain.ErrInvalidIDCard),
		errors.Is(err, domain.ErrNameRequired):
		writeErr(w, codeCertInvalid, err.Error())
	case errors.Is(err, domain.ErrNotMatched):
		writeErr(w, codeNotMatched, err.Error())
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
	case "verified", "org_admin": // 企业/机构认证与组织管理员在后续增量落地
		return false, nil
	default:
		return false, nil
	}
}

var (
	_ contract.Facade = (*Module)(nil)
	_ module.Module   = (*Module)(nil)
)
