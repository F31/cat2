// Package discovery R5 搜索与推荐模块：事件投影读模型、匹配预计算、六类检索与解释。
// 规则：只订阅事件（不 import 任何模块 contract 的 Facade）；读模型可由事件回放重建。
package discovery

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
	"catcube/internal/kernel/obs"
	"catcube/internal/kernel/settings"
	"catcube/internal/modules/discovery/contract"
	"catcube/internal/modules/discovery/internal/app"
	"catcube/internal/modules/discovery/internal/domain"
	"catcube/internal/modules/discovery/internal/infra"
	idcontract "catcube/internal/modules/identity/contract"
)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Identity idcontract.Facade
	Settings *settings.Registry
	Outbox   app.OutboxReader
}

// Module discovery 模块。
type Module struct {
	deps Deps
	svc  *app.Service
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "discovery" }
func (m *Module) Rank() module.Rank   { return module.RankR5 }
func (m *Module) DependsOn() []string { return []string{"identity"} }

func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{
		Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus,
		Settings: m.deps.Settings, Outbox: m.deps.Outbox,
	})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/search", Handler: m.handleSearch},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/matches", Handler: m.auth(m.handleFeed)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/matches/{ref}/explain", Handler: m.auth(m.handleExplain)},
	)
	r.EventSubs = append(r.EventSubs, m.svc.Subscriptions()...)
	r.Settings = append(r.Settings, app.Schemas...)
	r.Jobs = append(r.Jobs, module.JobDef{Name: "discovery.refresh", Interval: 30 * time.Minute, Run: func(ctx context.Context) error {
		_, err := m.svc.RefreshAll(ctx)
		return err
	}})
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约（只读）。
func (m *Module) Facade() contract.Facade { return m }

func (m *Module) Feed(ctx context.Context, userID int64, limit, offset int) ([]contract.Card, error) {
	return m.svc.Feed(ctx, userID, limit, offset)
}
func (m *Module) FeedFiltered(ctx context.Context, userID int64, typ string, limit, offset int) ([]contract.Card, error) {
	return m.svc.FeedFiltered(ctx, userID, typ, limit, offset)
}
func (m *Module) PublicFeed(ctx context.Context, typ string, limit, offset int) ([]contract.Card, error) {
	return m.svc.PublicFeed(ctx, typ, limit, offset)
}
func (m *Module) Search(ctx context.Context, q contract.SearchQuery) ([]contract.Card, error) {
	return m.svc.Search(ctx, q)
}
func (m *Module) Explain(ctx context.Context, userID int64, ref string) (contract.Explain, error) {
	return m.svc.Explain(ctx, userID, ref)
}

// Rebuild 投影回放重建（运维/测试）。
func (m *Module) Rebuild(ctx context.Context) (int, error) { return m.svc.Rebuild(ctx) }

// RefreshAll 重算全部画像候选（运维/测试）。
func (m *Module) RefreshAll(ctx context.Context) (int, error) { return m.svc.RefreshAll(ctx) }

// ---- HTTP 层 ----

// discovery 错误码（contracts/errors/README.md 16 段）。
const (
	codeParamInvalid = 1      // 000001
	codeUnauthorized = 2      // 000002
	codeNotFound     = 4      // 000004
	codeInternal     = 9      // 000009
	codeBadType      = 160001 // 160001 未知内容类型
	codeNoProfile    = 160002 // 160002 无画像
	codeNoCandidate  = 160003 // 160003 候选不存在
)

func (m *Module) handleSearch(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	res, err := m.svc.Search(req.Context(), contract.SearchQuery{
		Keyword: q.Get("q"), Type: q.Get("type"), Region: q.Get("region"),
		Industry: splitCSV(q.Get("industry")), Need: splitCSV(q.Get("need")), Offer: splitCSV(q.Get("offer")),
		Limit: limit,
	})
	if err != nil {
		writeDiscErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": res})
}

func (m *Module) handleFeed(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))
	res, err := m.svc.Feed(req.Context(), userIDFrom(req.Context()), limit, offset)
	if err != nil {
		writeDiscErr(w, err)
		return
	}
	obs.Default().Add("catcube_match_feed_total", float64(len(res)))
	writeData(w, map[string]any{"items": res})
}

func (m *Module) handleExplain(w http.ResponseWriter, req *http.Request) {
	obs.Default().Inc("catcube_match_click_total")
	ref := req.PathValue("ref")
	if _, _, ok := domain.SplitRef(ref); !ok {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	res, err := m.svc.Explain(req.Context(), userIDFrom(req.Context()), ref)
	if err != nil {
		writeDiscErr(w, err)
		return
	}
	writeData(w, res)
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

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeDiscErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrBadType):
		writeErr(w, codeBadType, err.Error())
	case errors.Is(err, domain.ErrNoProfile):
		writeErr(w, codeNoProfile, err.Error())
	case errors.Is(err, domain.ErrNoCandidate):
		writeErr(w, codeNoCandidate, err.Error())
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeNoProfile, codeNoCandidate:
		return http.StatusNotFound
	case codeParamInvalid, codeBadType:
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
	_ contract.Facade = (*Module)(nil)
	_ module.Module   = (*Module)(nil)
)
