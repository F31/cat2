// Package ops 运维模块（R0）：Outbox 死信查询与重放（p1 §6.2；p6 降级演练复用）。
// 只做后台运维操作，不承载业务。
package ops

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/featureflag"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/settings"
	idcontract "catcube/internal/modules/identity/contract"
)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Identity idcontract.Facade
	Outbox   eventbus.OutboxStore
	Bus      *eventbus.Bus
	Flags    *featureflag.Store
	Settings *settings.Registry
}

// Module ops 模块。
type Module struct {
	deps Deps
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "ops" }
func (m *Module) Rank() module.Rank   { return module.RankR1 }
func (m *Module) DependsOn() []string { return []string{"identity"} }

func (m *Module) Register(r *module.Registry) error {
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/dead-letters", Handler: m.auth(m.handleList), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/dead-letters/{eventID}/replay", Handler: m.auth(m.handleReplay), Admin: true},
		// 业务配置（配置三层第三层：后台改，热生效）
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/settings", Handler: m.auth(m.handleSettings), Admin: true},
		module.Route{Method: http.MethodPut, Path: "/api/admin/v1/settings/{key}", Handler: m.auth(m.handleSetSetting), Admin: true},
		// 特性开关（灰度发布）：后台配置 + 客户端评估
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/feature-flags", Handler: m.auth(m.handleFlags), Admin: true},
		module.Route{Method: http.MethodPut, Path: "/api/admin/v1/feature-flags/{key}", Handler: m.auth(m.handleSetFlag), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/flags", Handler: m.auth(m.handleMyFlags)},
	)
	return nil
}

func (m *Module) handleList(w http.ResponseWriter, req *http.Request) {
	events, err := m.deps.Outbox.ListDead(req.Context())
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	items := make([]map[string]any, 0, len(events))
	for _, e := range events {
		items = append(items, map[string]any{
			"event_id": e.ID, "topic": e.Topic, "space_id": e.SpaceID,
			"occurred_at": e.OccurredAt, "payload": json.RawMessage(e.Payload),
		})
	}
	writeData(w, map[string]any{"items": items})
}

// handleSettings 后台：业务配置列表（含声明与当前值）。
func (m *Module) handleSettings(w http.ResponseWriter, req *http.Request) {
	if m.deps.Settings == nil {
		writeData(w, map[string]any{"items": []any{}})
		return
	}
	items := make([]map[string]any, 0)
	for _, sc := range m.deps.Settings.Schemas() {
		cur, err := m.deps.Settings.Get(req.Context(), 1, sc.Key)
		value := sc.Default
		if err == nil {
			value = cur
		}
		items = append(items, map[string]any{
			"key": sc.Key, "type": string(sc.Type), "value": value,
			"default": sc.Default, "description": sc.Description, "sensitive": sc.Sensitive,
		})
	}
	writeData(w, map[string]any{"items": items})
}

// handleSetSetting 后台：修改业务配置（热生效；敏感项不回显明文）。
func (m *Module) handleSetSetting(w http.ResponseWriter, req *http.Request) {
	key := req.PathValue("key")
	if key == "" || m.deps.Settings == nil {
		writeErr(w, 1, "参数错误")
		return
	}
	if _, err := m.deps.Settings.GetSchema(key); err != nil {
		writeErr(w, 1, "未登记的配置项")
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, 1, "参数错误")
		return
	}
	op := userIDFrom(req.Context())
	if err := m.deps.Settings.Set(req.Context(), 1, key, body.Value, op); err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	writeData(w, map[string]any{"saved": true})
}

// handleFlags 后台：开关列表。
func (m *Module) handleFlags(w http.ResponseWriter, req *http.Request) {
	if m.deps.Flags == nil {
		writeData(w, map[string]any{"items": []any{}})
		return
	}
	list, err := m.deps.Flags.List(req.Context())
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, f := range list {
		items = append(items, map[string]any{"key": f.Key, "enabled": f.Enabled,
			"ratio": f.Rules.Ratio, "users": f.Rules.Users})
	}
	writeData(w, map[string]any{"items": items})
}

// handleSetFlag 后台：配置开关（enabled/ratio/users）。
func (m *Module) handleSetFlag(w http.ResponseWriter, req *http.Request) {
	key := req.PathValue("key")
	if key == "" || m.deps.Flags == nil {
		writeErr(w, 1, "参数错误")
		return
	}
	var body struct {
		Enabled bool    `json:"enabled"`
		Ratio   int     `json:"ratio"`
		Users   []int64 `json:"users"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, 1, "参数错误")
		return
	}
	if body.Ratio < 0 || body.Ratio > 100 {
		writeErr(w, 1, "ratio 需在 0-100")
		return
	}
	if err := m.deps.Flags.Set(req.Context(), featureflag.Flag{Key: key, Enabled: body.Enabled,
		Rules: featureflag.Rules{Users: body.Users, Ratio: body.Ratio}}); err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	writeData(w, map[string]any{"saved": true})
}

// handleMyFlags 客户端：按当前用户评估全部开关（灰度放量入口）。
func (m *Module) handleMyFlags(w http.ResponseWriter, req *http.Request) {
	out := map[string]bool{}
	if m.deps.Flags != nil {
		if list, err := m.deps.Flags.List(req.Context()); err == nil {
			uid := userIDFrom(req.Context())
			for _, f := range list {
				out[f.Key] = featureflag.Eval(f, uid)
			}
		}
	}
	writeData(w, map[string]any{"flags": out})
}

func (m *Module) handleReplay(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("eventID")
	if id == "" {
		writeErr(w, 1, "参数错误")
		return
	}
	if err := m.deps.Outbox.Replay(req.Context(), id); err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	writeData(w, map[string]any{"replayed": true})
}

func (m *Module) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			writeErr(w, 2, "未登录")
			return
		}
		uid, ver, err := m.deps.Identity.Authenticate(req.Context(), token)
		if err != nil {
			writeErr(w, 2, "登录已失效")
			return
		}
		brief, err := m.deps.Identity.UserBrief(req.Context(), uid)
		if err != nil || brief.AuthzVer != ver || (brief.Status != 0 && brief.Status != 1) {
			writeErr(w, 2, "登录已失效")
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

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

func writeErr(w http.ResponseWriter, code int, message string) {
	status := http.StatusInternalServerError
	switch code {
	case 1:
		status = http.StatusBadRequest
	case 2:
		status = http.StatusUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}
