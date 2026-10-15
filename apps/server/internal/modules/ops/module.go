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
	"catcube/internal/kernel/module"
	idcontract "catcube/internal/modules/identity/contract"
)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Identity idcontract.Facade
	Outbox   eventbus.OutboxStore
	Bus      *eventbus.Bus
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
