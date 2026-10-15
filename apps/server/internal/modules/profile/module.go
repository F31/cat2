// Package profile R1 名片与画像模块：一个账号一张名片（个人/组织）、三层画像、
// 对外展示设置、名片分享；注册 ReviewSubjectProvider 与 EntityResolver 扩展点。
package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/sensitive"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/modules/profile/contract"
	"catcube/internal/modules/profile/internal/app"
	"catcube/internal/modules/profile/internal/domain"
	"catcube/internal/modules/profile/internal/infra"
	dictcontract "catcube/internal/shared/dict/contract"
	"catcube/internal/shared/entityref"
	mediacontract "catcube/internal/shared/media/contract"
)

// Deps 装配依赖。
type Deps struct {
	Pool           *db.Pool
	Bus            *eventbus.Bus
	Media          mediacontract.Facade
	Dict           dictcontract.Facade
	Identity       idcontract.Facade
	Encryptor      *sensitive.Encryptor
	EntityRegistry *entityref.Registry
}

// Module profile 模块。
type Module struct {
	deps  Deps
	svc   *app.Service
	store *infra.Store
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "profile" }
func (m *Module) Rank() module.Rank   { return module.RankR1 }
func (m *Module) DependsOn() []string { return []string{"dict", "media", "identity"} }

// Register 注册路由、事件订阅与扩展点。
func (m *Module) Register(r *module.Registry) error {
	m.store = infra.NewStore(m.deps.Pool)
	m.svc = app.New(app.Options{
		Repo: m.store, Pool: m.deps.Pool, Bus: m.deps.Bus, Media: m.deps.Media,
		Dict: m.deps.Dict, Identity: m.deps.Identity, Encrypt: m.deps.Encryptor,
	})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cards/me", Handler: m.auth(m.handleGetMe)},
		module.Route{Method: http.MethodPut, Path: "/api/mp/v1/cards/me", Handler: m.auth(m.handlePutMe)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cards/me/completion", Handler: m.auth(m.handleCompletion)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/cards/me/share", Handler: m.auth(m.handleShare)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/cards/{ref}", Handler: m.handlePublicCard},
	)
	// 订阅注册事件：建默认名片（第一层画像）
	r.EventSubs = append(r.EventSubs, eventbus.Subscription{
		Topic: idcontract.EventUserRegistered, Handler: m.svc.HandleUserRegistered,
	})
	// 订阅认证通过（verify R2 事件，topic 字面量，不 import 高阶 contract）：锁定认证主体字段
	r.EventSubs = append(r.EventSubs, eventbus.Subscription{
		Topic: "verify.CertificationApproved.v1", Handler: m.svc.HandleCertificationApproved,
	})
	// 扩展点：EntityResolver（user/org）
	if m.deps.EntityRegistry != nil {
		m.deps.EntityRegistry.Register("user", m)
		m.deps.EntityRegistry.Register("org", m)
	}
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

// ---- contract.Facade 实现 ----

func (m *Module) CardOf(ctx context.Context, userID int64) (contract.Card, error) {
	return m.svc.PublicCard(ctx, userID)
}

func (m *Module) CompletionOf(ctx context.Context, userID int64) (contract.Completion, error) {
	return m.svc.Completion(ctx, userID)
}

func (m *Module) OrgBrief(ctx context.Context, userID int64) (contract.OrgBrief, error) {
	return m.svc.OrgBrief(ctx, userID)
}

// ContactPhone 返回联系方式明文（会员门槛由 bff 判定）。
func (m *Module) ContactPhone(ctx context.Context, userID int64) (string, error) {
	return m.svc.ContactPhone(ctx, userID)
}

// ---- trust.ReviewSubjectProvider 实现（注册到 trust）----

func (m *Module) SubjectType() string { return m.svc.SubjectType() }
func (m *Module) Extract(ctx context.Context, subjectID int64) (string, []string, error) {
	return m.svc.Extract(ctx, subjectID)
}
func (m *Module) OwnerOf(_ context.Context, subjectID int64) (int64, error) { return subjectID, nil }
func (m *Module) ApplyVerdict(ctx context.Context, subjectID int64, pass bool, reason string) error {
	return m.svc.ApplyVerdict(ctx, subjectID, pass, reason)
}

// ---- entityref.Resolver 实现（user/org）----

func (m *Module) Resolve(ctx context.Context, ref entityref.Ref) (entityref.Info, error) {
	if ref.Type != "user" && ref.Type != "org" {
		return entityref.Info{}, fmt.Errorf("profile: 不支持的引用类型 %q", ref.Type)
	}
	b, err := m.svc.OrgBrief(ctx, ref.ID)
	if err != nil {
		return entityref.Info{}, err
	}
	return entityref.Info{Title: b.Name, Status: "active", Visibility: "platform"}, nil
}

// ---- HTTP 层 ----

// profile 错误码（contracts/errors/README.md 06 段）。
const (
	codeParamInvalid = 1     // 000001
	codeUnauthorized = 2     // 000002
	codeForbidden    = 3     // 000003
	codeInternal     = 9     // 000009
	codeCardNotFound = 60001 // 060001
	codeCardInvalid  = 60002 // 060002
	codeFieldLocked  = 60003 // 060003
	codeTagInvalid   = 60004 // 060004
)

func (m *Module) handleGetMe(w http.ResponseWriter, req *http.Request) {
	card, err := m.svc.GetMyCard(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeCardErr(w, err)
		return
	}
	writeData(w, card)
}

func (m *Module) handlePutMe(w http.ResponseWriter, req *http.Request) {
	var in app.UpdateInput
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	card, err := m.svc.UpdateCard(req.Context(), userIDFrom(req.Context()), in)
	if err != nil {
		writeCardErr(w, err)
		return
	}
	writeData(w, card)
}

func (m *Module) handleCompletion(w http.ResponseWriter, req *http.Request) {
	c, err := m.svc.Completion(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeCardErr(w, err)
		return
	}
	writeData(w, c)
}

func (m *Module) handleShare(w http.ResponseWriter, req *http.Request) {
	uid := userIDFrom(req.Context())
	card, err := m.svc.GetMyCard(req.Context(), uid)
	if err != nil {
		writeCardErr(w, err)
		return
	}
	writeData(w, map[string]any{
		"share_path": fmt.Sprintf("/pages/card/index?ref=user:%d", uid),
		"title":      card.Name,
	})
}

// handlePublicCard 他人名片（游客可访问；按对外展示设置过滤）。
func (m *Module) handlePublicCard(w http.ResponseWriter, req *http.Request) {
	refType, id, err := parseRef(req.PathValue("ref"))
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if refType != "user" && refType != "org" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	card, err := m.svc.PublicCard(req.Context(), id)
	if err != nil {
		writeCardErr(w, err)
		return
	}
	writeData(w, card)
}

// auth 鉴权中间件：identity Facade 验签 + authz_ver 陈旧窗口比对 + 账号状态。
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
			writeErr(w, codeForbidden, "账号不可用")
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

func parseRef(ref string) (string, int64, error) {
	i := strings.LastIndex(ref, ":")
	if i <= 0 {
		return "", 0, fmt.Errorf("profile: ref 格式应为 type:id")
	}
	id, err := strconv.ParseInt(ref[i+1:], 10, 64)
	if err != nil {
		return "", 0, err
	}
	return ref[:i], id, nil
}

func writeCardErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound) || errors.Is(err, infra.ErrNotFound):
		writeErr(w, codeCardNotFound, "名片不存在")
	case errors.Is(err, domain.ErrNameRequired), errors.Is(err, domain.ErrIntroTooLong),
		errors.Is(err, domain.ErrStrengthTooLong):
		writeErr(w, codeCardInvalid, err.Error())
	case errors.Is(err, domain.ErrInvalidTag):
		writeErr(w, codeTagInvalid, "标签非法")
	case errors.Is(err, domain.ErrFieldLocked):
		writeErr(w, codeFieldLocked, "认证信息不可自行修改，请走撤销重申请")
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeForbidden, codeFieldLocked:
		return http.StatusForbidden
	case codeCardNotFound:
		return http.StatusNotFound
	case codeParamInvalid, codeCardInvalid, codeTagInvalid:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeErr(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus(code))
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}

var (
	_ contract.Facade    = (*Module)(nil)
	_ entityref.Resolver = (*Module)(nil)
	_ module.Module      = (*Module)(nil)
)
