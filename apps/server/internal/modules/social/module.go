// Package social R1 关注收藏模块：关注/取关、收藏（六类分 Tab）、关注未读；
// 目标统一 EntityRef，展示信息经 EntityResolver 回填。不做排行榜与粉丝增长提醒。
package social

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/modules/social/contract"
	"catcube/internal/modules/social/internal/app"
	"catcube/internal/modules/social/internal/domain"
	"catcube/internal/modules/social/internal/infra"
	"catcube/internal/shared/entityref"
)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Identity idcontract.Facade
	Resolver *entityref.Registry
}

// Module social 模块。
type Module struct {
	deps Deps
	svc  *app.Service
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "social" }
func (m *Module) Rank() module.Rank   { return module.RankR1 }
func (m *Module) DependsOn() []string { return []string{"identity"} }

func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus, Resolver: m.deps.Resolver})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/follows", Handler: m.auth(m.handleFollow)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/follows", Handler: m.auth(m.handleFollowing)},
		module.Route{Method: http.MethodDelete, Path: "/api/mp/v1/follows/{ref}", Handler: m.auth(m.handleUnfollow)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/followers", Handler: m.auth(m.handleFollowers)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/favorites", Handler: m.auth(m.handleFavorite)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/favorites", Handler: m.auth(m.handleFavorites)},
		module.Route{Method: http.MethodDelete, Path: "/api/mp/v1/favorites/{ref}", Handler: m.auth(m.handleUnfavorite)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/follow-unread", Handler: m.auth(m.handleUnread)},
	)
	// 关注=订阅：订阅内容发布（topic 按契约，social R1 不 import content R2）
	r.EventSubs = append(r.EventSubs, eventbus.Subscription{
		Topic: app.ContentPublishedTopic, Handler: m.svc.HandleContentPublished,
	})
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

func (m *Module) Follow(ctx context.Context, followerID int64, target string) error {
	return m.svc.Follow(ctx, followerID, target)
}
func (m *Module) Unfollow(ctx context.Context, followerID int64, target string) error {
	return m.svc.Unfollow(ctx, followerID, target)
}
func (m *Module) IsFollowing(ctx context.Context, followerID int64, target string) (bool, error) {
	return m.svc.IsFollowing(ctx, followerID, target)
}
func (m *Module) Following(ctx context.Context, userID int64) ([]contract.FollowItem, error) {
	return m.svc.Following(ctx, userID)
}
func (m *Module) Followers(ctx context.Context, userID int64) ([]contract.FollowItem, error) {
	return m.svc.Followers(ctx, userID)
}
func (m *Module) Favorite(ctx context.Context, userID int64, target string) error {
	return m.svc.Favorite(ctx, userID, target)
}
func (m *Module) Unfavorite(ctx context.Context, userID int64, target string) error {
	return m.svc.Unfavorite(ctx, userID, target)
}
func (m *Module) Favorites(ctx context.Context, userID int64, kind string) ([]contract.FavoriteItem, error) {
	return m.svc.Favorites(ctx, userID, kind)
}
func (m *Module) Unread(ctx context.Context, userID int64) (int, error) {
	return m.svc.Unread(ctx, userID)
}

// ---- HTTP 层 ----

// social 错误码（contracts/errors/README.md 09 段）。
const (
	codeParamInvalid = 1     // 000001
	codeUnauthorized = 2     // 000002
	codeInternal     = 9     // 000009
	codeBadTarget    = 90001 // 090001 目标引用非法
	codeBadKind      = 90002 // 090002 不支持的收藏分类
)

type targetBody struct {
	TargetRef string `json:"target_ref"`
}

func (m *Module) handleFollow(w http.ResponseWriter, req *http.Request) {
	body, ok := decodeTarget(w, req)
	if !ok {
		return
	}
	if err := m.svc.Follow(req.Context(), userIDFrom(req.Context()), body.TargetRef); err != nil {
		writeSocialErr(w, err)
		return
	}
	writeData(w, map[string]any{"following": true})
}

func (m *Module) handleUnfollow(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.Unfollow(req.Context(), userIDFrom(req.Context()), req.PathValue("ref")); err != nil {
		writeSocialErr(w, err)
		return
	}
	writeData(w, map[string]any{"following": false})
}

func (m *Module) handleFollowing(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.Following(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeSocialErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleFollowers(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.Followers(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeSocialErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleFavorite(w http.ResponseWriter, req *http.Request) {
	body, ok := decodeTarget(w, req)
	if !ok {
		return
	}
	if err := m.svc.Favorite(req.Context(), userIDFrom(req.Context()), body.TargetRef); err != nil {
		writeSocialErr(w, err)
		return
	}
	writeData(w, map[string]any{"favorited": true})
}

func (m *Module) handleUnfavorite(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.Unfavorite(req.Context(), userIDFrom(req.Context()), req.PathValue("ref")); err != nil {
		writeSocialErr(w, err)
		return
	}
	writeData(w, map[string]any{"favorited": false})
}

func (m *Module) handleFavorites(w http.ResponseWriter, req *http.Request) {
	kind := req.URL.Query().Get("type")
	if !domain.ValidKind(kind) {
		writeErr(w, codeBadKind, "不支持的收藏分类")
		return
	}
	items, err := m.svc.Favorites(req.Context(), userIDFrom(req.Context()), kind)
	if err != nil {
		writeSocialErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleUnread(w http.ResponseWriter, req *http.Request) {
	n, err := m.svc.Unread(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeSocialErr(w, err)
		return
	}
	writeData(w, map[string]any{"unread": n})
}

func decodeTarget(w http.ResponseWriter, req *http.Request) (targetBody, bool) {
	var body targetBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.TargetRef == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return body, false
	}
	return body, true
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

func writeSocialErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidTarget):
		writeErr(w, codeBadTarget, "目标引用非法")
	case errors.Is(err, domain.ErrKindRequired):
		writeErr(w, codeBadKind, "不支持的收藏分类")
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeParamInvalid, codeBadTarget, codeBadKind:
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
