// Package content R2 内容模块：四类内容（ContentKind 插件）、信封表 + 类型子表、状态机 #2、
// 发布门槛与会员可见字段；注册 EntityResolver 与 ReviewSubjectProvider。
package content

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
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/sensitive"
	contentcontract "catcube/internal/modules/content/contract"
	"catcube/internal/modules/content/internal/app"
	"catcube/internal/modules/content/internal/domain"
	"catcube/internal/modules/content/internal/infra"
	"catcube/internal/modules/content/internal/kind"
	idcontract "catcube/internal/modules/identity/contract"
	trustcontract "catcube/internal/modules/trust/contract"
	dictcontract "catcube/internal/shared/dict/contract"
	"catcube/internal/shared/entityref"
	mediacontract "catcube/internal/shared/media/contract"
)

// Deps 装配依赖。
type Deps struct {
	Pool           *db.Pool
	Bus            *eventbus.Bus
	Dict           dictcontract.Facade
	Trust          trustcontract.Facade
	Media          mediacontract.Facade
	Identity       idcontract.Facade
	Policy         *policy.Engine
	Encryptor      *sensitive.Encryptor
	EntityRegistry *entityref.Registry
}

// Module content 模块。
type Module struct {
	deps  Deps
	kinds *kind.Registry
	svc   *app.Service
}

func New(deps Deps) *Module { return &Module{deps: deps, kinds: kind.DefaultRegistry()} }

func (m *Module) Name() string        { return "content" }
func (m *Module) Rank() module.Rank   { return module.RankR2 }
func (m *Module) DependsOn() []string { return []string{"dict", "trust", "media", "identity"} }

// Register 注册路由与扩展点。
func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{
		Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus, Kinds: m.kinds,
		Dict: m.deps.Dict, Trust: m.deps.Trust, Media: m.deps.Media, Policy: m.deps.Policy, Encrypt: m.deps.Encryptor,
	})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/contents", Handler: m.auth(m.handlePublish)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/contents/mine", Handler: m.auth(m.handleMine)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/contents/{id}/submit", Handler: m.auth(m.handleSubmit)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/contents/{id}", Handler: m.handleGet},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/contents/{id}/close", Handler: m.auth(m.handleClose)},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/contents", Handler: m.auth(m.handleAdminList), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/contents/{id}/takedown", Handler: m.auth(m.handleAdminTakedown), Admin: true},
	)
	if m.deps.EntityRegistry != nil {
		for _, k := range []string{contentcontract.KindProject, contentcontract.KindProduct, contentcontract.KindMeeting, contentcontract.KindEvent} {
			m.deps.EntityRegistry.Register("content."+k, m)
		}
	}
	// 订阅 trust 举报核实结论：下架内容（trust 不直接改 cnt_item.status）
	r.EventSubs = append(r.EventSubs, eventbus.Subscription{
		Topic: trustcontract.EventReportConfirmed, Handler: m.svc.HandleReportConfirmed,
	})
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contentcontract.Facade { return m }

func (m *Module) Publish(ctx context.Context, userID int64, in contentcontract.PublishInput) (contentcontract.Item, error) {
	return m.svc.Publish(ctx, userID, in)
}
func (m *Module) Submit(ctx context.Context, userID, itemID int64) (contentcontract.Item, error) {
	return m.svc.Submit(ctx, userID, itemID)
}
func (m *Module) Get(ctx context.Context, viewerID, itemID int64) (contentcontract.Item, error) {
	return m.svc.Get(ctx, viewerID, itemID)
}
func (m *Module) ListByOwner(ctx context.Context, ownerID int64, kindName string) ([]contentcontract.Item, error) {
	return m.svc.ListByOwner(ctx, ownerID, kindName)
}
func (m *Module) Close(ctx context.Context, userID, itemID int64) (contentcontract.Item, error) {
	return m.svc.Close(ctx, userID, itemID)
}

// ---- entityref.Resolver（content.<kind>）----

func (m *Module) Resolve(ctx context.Context, ref entityref.Ref) (entityref.Info, error) {
	if !strings.HasPrefix(ref.Type, "content.") {
		return entityref.Info{}, fmt.Errorf("content: 不支持的引用类型 %q", ref.Type)
	}
	it, err := m.svc.Get(ctx, 0, ref.ID)
	if err != nil {
		return entityref.Info{}, err
	}
	return entityref.Info{Title: it.Title, Status: statusWord(it.Status), Visibility: "platform"}, nil
}

// ProviderFor 返回某 kind 的 trust.ReviewSubjectProvider（wire 注册进 trust）。
func (m *Module) ProviderFor(kindName string) trustcontract.ReviewSubjectProvider {
	return reviewProvider{kind: kindName, m: m}
}

type reviewProvider struct {
	kind string
	m    *Module
}

func (p reviewProvider) SubjectType() string { return "content." + p.kind }
func (p reviewProvider) Extract(ctx context.Context, subjectID int64) (string, []string, error) {
	return p.m.svc.ReviewText(ctx, subjectID)
}
func (p reviewProvider) OwnerOf(ctx context.Context, subjectID int64) (int64, error) {
	return p.m.svc.OwnerOf(ctx, subjectID)
}
func (p reviewProvider) ApplyVerdict(_ context.Context, _ int64, _ bool, _ string) error { return nil }

func statusWord(s int16) string {
	switch s {
	case contentcontract.StatusPublished:
		return "published"
	case contentcontract.StatusTakenDown:
		return "taken_down"
	default:
		return "reviewing"
	}
}

// ---- HTTP 层 ----

// content 错误码（contracts/errors/README.md 11 段）。
const (
	codeParamInvalid   = 1      // 000001
	codeUnauthorized   = 2      // 000002
	codeForbidden      = 3      // 000003
	codeNotFound       = 4      // 000004
	codeInternal       = 9      // 000009
	codeItemNotFound   = 110001 // 110001 内容不存在
	codeNoPermission   = 110002 // 110002 无权操作
	codeFieldInvalid   = 110003 // 110003 字段校验失败
	codeRealnameNeeded = 110004 // 110004 发布需实名
	codeKindUnknown    = 110005 // 110005 未知内容类型
)

func (m *Module) handlePublish(w http.ResponseWriter, req *http.Request) {
	var in contentcontract.PublishInput
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	item, err := m.svc.Publish(req.Context(), userIDFrom(req.Context()), in)
	if err != nil {
		writeContentErr(w, err)
		return
	}
	writeData(w, item)
}

func (m *Module) handleSubmit(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	item, err := m.svc.Submit(req.Context(), userIDFrom(req.Context()), id)
	if err != nil {
		writeContentErr(w, err)
		return
	}
	writeData(w, item)
}

func (m *Module) handleClose(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	item, err := m.svc.Close(req.Context(), userIDFrom(req.Context()), id)
	if err != nil {
		writeContentErr(w, err)
		return
	}
	writeData(w, item)
}

// ---- 后台管理（RBAC 细化在 p4）----

func (m *Module) handleAdminList(w http.ResponseWriter, req *http.Request) {
	status := int16(-1)
	if s := req.URL.Query().Get("status"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			status = int16(v)
		}
	}
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	items, err := m.svc.AdminList(req.Context(), req.URL.Query().Get("kind"), status, limit)
	if err != nil {
		writeContentErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleAdminTakedown(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	item, err := m.svc.AdminTakedown(req.Context(), id, body.Reason)
	if err != nil {
		writeContentErr(w, err)
		return
	}
	writeData(w, item)
}

func (m *Module) handleGet(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	viewer := int64(0)
	if token := bearer(req); token != "" {
		if uid, ver, err := m.deps.Identity.Authenticate(req.Context(), token); err == nil {
			if b, err := m.deps.Identity.UserBrief(req.Context(), uid); err == nil && b.AuthzVer == ver {
				viewer = uid
			}
		}
	}
	item, err := m.svc.Get(req.Context(), viewer, id)
	if err != nil {
		writeContentErr(w, err)
		return
	}
	writeData(w, item)
}

func (m *Module) handleMine(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.ListByOwner(req.Context(), userIDFrom(req.Context()), req.URL.Query().Get("kind"))
	if err != nil {
		writeContentErr(w, err)
		return
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

func pathID(req *http.Request) (int64, error) {
	return strconv.ParseInt(req.PathValue("id"), 10, 64)
}

func writeContentErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound) || errors.Is(err, infra.ErrNotFound):
		writeErr(w, codeItemNotFound, "内容不存在")
	case errors.Is(err, domain.ErrForbidden):
		writeErr(w, codeNoPermission, "无权操作该内容")
	case errors.Is(err, domain.ErrNotRealname):
		writeErr(w, codeRealnameNeeded, "发布需先完成实名认证")
	case errors.Is(err, domain.ErrTitleRequired), errors.Is(err, domain.ErrTitleTooLong):
		writeErr(w, codeFieldInvalid, err.Error())
	case errors.Is(err, domain.ErrKindUnknown):
		writeErr(w, codeKindUnknown, "未知内容类型")
	default:
		var fe *kind.FieldError
		if errors.As(err, &fe) {
			writeErr(w, codeFieldInvalid, err.Error())
			return
		}
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeForbidden, codeRealnameNeeded:
		return http.StatusForbidden
	case codeItemNotFound:
		return http.StatusNotFound
	case codeParamInvalid, codeFieldInvalid, codeKindUnknown:
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
	_ contentcontract.Facade = (*Module)(nil)
	_ entityref.Resolver     = (*Module)(nil)
	_ module.Module          = (*Module)(nil)
)
