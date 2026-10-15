// Package notice R6 通知中心：模板数据驱动（事件→接收人→文案→渠道→合并），业务模块从不直接发通知。
package notice

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
	circlecontract "catcube/internal/modules/circle/contract"
	idcontract "catcube/internal/modules/identity/contract"
	noticecontract "catcube/internal/modules/notice/contract"
	"catcube/internal/modules/notice/internal/app"
	"catcube/internal/modules/notice/internal/domain"
	"catcube/internal/modules/notice/internal/infra"
)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Identity idcontract.Facade
	Circle   circlecontract.Facade
}

// Module notice 模块。
type Module struct {
	deps Deps
	svc  *app.Service
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "notice" }
func (m *Module) Rank() module.Rank   { return module.RankR6 }
func (m *Module) DependsOn() []string { return []string{"identity", "circle"} }

// notice 错误码（contracts/errors/README.md 17 段）。
const (
	codeParamInvalid = 1
	codeUnauthorized = 2
	codeInternal     = 9
	codeNotFound     = 170001
	codeQuotaShort   = 170002
	codeTemplate     = 170003
)

func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus, Circle: m.deps.Circle})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/notices", Handler: m.auth(m.handleList)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/notices/{id}/read", Handler: m.auth(m.handleRead)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/notice-preferences", Handler: m.auth(m.handlePreferences)},
		module.Route{Method: http.MethodPut, Path: "/api/mp/v1/notice-preferences", Handler: m.auth(m.handleSetPreference)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/subscribe-quota/apply", Handler: m.auth(m.handleApplyQuota)},
	)
	// 订阅全部业务事件（事件驱动触达）
	for _, topic := range app.Topics() {
		r.EventSubs = append(r.EventSubs, eventbus.Subscription{Topic: topic, Handler: m.svc.Handle})
	}
	r.Jobs = append(r.Jobs,
		module.JobDef{Name: "notice.seed_templates", Interval: 10 * time.Minute, Run: m.svc.Register},
		module.JobDef{Name: "notice.digest", Interval: time.Minute, Run: m.svc.RunDigest},
	)
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() noticecontract.Facade { return m }

func (m *Module) List(ctx context.Context, userID int64, unreadOnly bool, limit int) ([]noticecontract.Notice, error) {
	return m.svc.List(ctx, userID, unreadOnly, limit)
}
func (m *Module) UnreadCount(ctx context.Context, userID int64) (int, error) {
	return m.svc.UnreadCount(ctx, userID)
}
func (m *Module) MarkRead(ctx context.Context, userID, noticeID int64) error {
	return m.svc.MarkRead(ctx, userID, noticeID)
}
func (m *Module) Preferences(ctx context.Context, userID int64) ([]noticecontract.Preference, error) {
	return m.svc.Preferences(ctx, userID)
}
func (m *Module) SetPreference(ctx context.Context, userID int64, code string, enabled bool) error {
	return m.svc.SetPreference(ctx, userID, code, enabled)
}
func (m *Module) ApplyQuota(ctx context.Context, userID int64, code string, n int) error {
	return m.svc.ApplyQuota(ctx, userID, code, n)
}

// RunDigest 手动触发日结（运维入口）。
func (m *Module) RunDigest(ctx context.Context) error { return m.svc.RunDigest(ctx) }

// SeedTemplates 手动登记模板（运维入口；定时由 notice.seed_templates Job 调用）。
func (m *Module) SeedTemplates(ctx context.Context) error { return m.svc.Register(ctx) }

// ---- HTTP ----

func (m *Module) handleList(w http.ResponseWriter, req *http.Request) {
	unreadOnly := req.URL.Query().Get("unread") == "1"
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	items, err := m.svc.List(req.Context(), userIDFrom(req.Context()), unreadOnly, limit)
	if err != nil {
		writeNoticeErr(w, err)
		return
	}
	unread, _ := m.svc.UnreadCount(req.Context(), userIDFrom(req.Context()))
	writeData(w, map[string]any{"items": items, "unread": unread})
}

func (m *Module) handleRead(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.MarkRead(req.Context(), userIDFrom(req.Context()), id); err != nil {
		writeNoticeErr(w, err)
		return
	}
	writeData(w, map[string]any{"read": true})
}

func (m *Module) handlePreferences(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.Preferences(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeNoticeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleSetPreference(w http.ResponseWriter, req *http.Request) {
	var body struct {
		TemplateCode string `json:"template_code"`
		Enabled      bool   `json:"enabled"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.TemplateCode == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.SetPreference(req.Context(), userIDFrom(req.Context()), body.TemplateCode, body.Enabled); err != nil {
		writeNoticeErr(w, err)
		return
	}
	writeData(w, map[string]any{"saved": true})
}

func (m *Module) handleApplyQuota(w http.ResponseWriter, req *http.Request) {
	var body struct {
		TemplateCode string `json:"template_code"`
		N            int    `json:"n"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.TemplateCode == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.ApplyQuota(req.Context(), userIDFrom(req.Context()), body.TemplateCode, body.N); err != nil {
		writeNoticeErr(w, err)
		return
	}
	writeData(w, map[string]any{"applied": true})
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

func writeNoticeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeErr(w, codeNotFound, err.Error())
	case errors.Is(err, domain.ErrTemplateNotFound):
		writeErr(w, codeTemplate, err.Error())
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeNotFound, codeTemplate:
		return http.StatusNotFound
	case codeParamInvalid:
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

var _ noticecontract.Facade = (*Module)(nil)
