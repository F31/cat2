// Package circle R4 圈子模块：生命周期、加入四路、圈内发布与挂载、到猫立方里办。
package circle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/modules/circle/contract"
	"catcube/internal/modules/circle/internal/app"
	"catcube/internal/modules/circle/internal/domain"
	"catcube/internal/modules/circle/internal/infra"
	contentcontract "catcube/internal/modules/content/contract"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/shared/entityref"
)

// Deps 装配依赖。
type Deps struct {
	Pool      *db.Pool
	Bus       *eventbus.Bus
	Identity  idcontract.Facade
	Policy    *policy.Engine
	Encryptor *sensitive.Encryptor
	Entities  *entityref.Registry
	Content   contentcontract.Facade
	Cube      cubefacade
}

// Module circle 模块。
type Module struct {
	deps Deps
	svc  *app.Service
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "circle" }
func (m *Module) Rank() module.Rank   { return module.RankR4 }
func (m *Module) DependsOn() []string { return []string{"identity", "content"} }

// circle 错误码（contracts/errors/README.md 14 段）。
const (
	codeParamInvalid  = 1
	codeUnauthorized  = 2
	codeInternal      = 9
	codeNotFound      = 140001
	codeNoPermission  = 140002
	codeNotRealname   = 140003
	codeDuration      = 140004
	codeCircleLimit   = 140005
	codeInviteBad     = 140006
	codeInvalidInput  = 140007
	codePostQuota     = 140008
	codeAnnounceQuota = 140009
	codeMountLimit    = 140010
	codeEnded         = 140012
)

func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{
		Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus,
		Policy: m.deps.Policy, Identity: m.deps.Identity, Encryptor: m.deps.Encryptor,
		Content: m.deps.Content, Cube: m.deps.Cube,
	})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles", Handler: m.auth(m.handleCreate)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles", Handler: m.auth(m.handleListMine)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}", Handler: m.handleDetail},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/apply", Handler: m.auth(m.handleApply)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/invite", Handler: m.auth(m.handleInvite)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/join", Handler: m.auth(m.handleJoin)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}/members", Handler: m.auth(m.handleMembers)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/dissolve", Handler: m.auth(m.handleDissolve)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/convert", Handler: m.auth(m.handleConvert)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/demands", Handler: m.auth(m.handlePublishDemand)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}/demands", Handler: m.handleListDemands},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/demands/{did}/renew", Handler: m.auth(m.handleRenewDemand)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/feeds", Handler: m.auth(m.handlePublishFeed)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/mounts", Handler: m.auth(m.handleMount)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}/mounts", Handler: m.handleListMounts},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/mounts/{mid}/decide", Handler: m.auth(m.handleDecideMount)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/demands/{did}/to-cube", Handler: m.auth(m.handleToCube)},
	)
	// 订阅 cube.ProjectCreated：来源圈条目回写「已转入猫立方办」
	r.EventSubs = append(r.EventSubs, eventbus.Subscription{
		Topic: "cube.ProjectCreated.v1", Handler: func(ctx context.Context, e eventbus.Event) error {
			return m.svc.HandleCubeProjectCreated(ctx, e.Payload)
		},
	})
	r.AuditPoints = append(r.AuditPoints,
		audit.Point{Action: "circle:dissolve", Path: "/api/mp/v1/circles/{id}/dissolve", Required: true, LogFailed: true, Module: "circle"},
	)
	r.StateMachines = append(r.StateMachines, domain.MachineCircle, domain.MachineDemand)
	r.Jobs = append(r.Jobs,
		module.JobDef{Name: "circle.end_due", Interval: time.Hour, Run: m.svc.EndDue},
		module.JobDef{Name: "circle.expire_demands", Interval: time.Hour, Run: m.svc.ExpireDemands},
	)
	if m.deps.Entities != nil {
		m.deps.Entities.Register("circle", m)
	}
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

func (m *Module) Brief(ctx context.Context, circleID int64) (contract.CircleBrief, error) {
	c, err := m.svc.Brief(ctx, circleID)
	return c, err
}
func (m *Module) RoleOf(ctx context.Context, circleID, userID int64) (int16, error) {
	return m.svc.RoleOf(ctx, circleID, userID)
}
func (m *Module) IsMember(ctx context.Context, circleID, userID int64) (bool, error) {
	return m.svc.IsMember(ctx, circleID, userID)
}
func (m *Module) ListMine(ctx context.Context, userID int64) ([]contract.CircleBrief, error) {
	return m.svc.ListMine(ctx, userID)
}
func (m *Module) Detail(ctx context.Context, viewerID, circleID int64) (contract.CircleDetail, error) {
	return m.svc.Detail(ctx, viewerID, circleID)
}
func (m *Module) Create(ctx context.Context, userID int64, in contract.CreateInput) (contract.CircleBrief, error) {
	return m.svc.Create(ctx, userID, in)
}
func (m *Module) Apply(ctx context.Context, circleID, userID int64) error {
	return m.svc.Apply(ctx, circleID, userID)
}
func (m *Module) ApproveApply(ctx context.Context, circleID, ownerID, applicantID int64) error {
	return m.svc.ApproveApply(ctx, circleID, ownerID, applicantID)
}
func (m *Module) InviteCode(ctx context.Context, circleID, userID int64) (string, time.Time, error) {
	return m.svc.InviteCode(ctx, circleID, userID)
}
func (m *Module) JoinByInvite(ctx context.Context, userID int64, code string) (contract.CircleBrief, error) {
	return m.svc.JoinByInvite(ctx, userID, code)
}
func (m *Module) Members(ctx context.Context, circleID int64) ([]int64, error) {
	return m.svc.Members(ctx, circleID)
}
func (m *Module) Dissolve(ctx context.Context, circleID, userID int64) error {
	return m.svc.Dissolve(ctx, circleID, userID)
}
func (m *Module) ConvertToPermanent(ctx context.Context, circleID, userID int64) error {
	return m.svc.ConvertToPermanent(ctx, circleID, userID)
}
func (m *Module) EndDue(ctx context.Context) error { return m.svc.EndDue(ctx) }
func (m *Module) PublishDemand(ctx context.Context, circleID, userID int64, in contract.DemandInput) (contract.Demand, error) {
	return m.svc.PublishDemand(ctx, circleID, userID, in)
}
func (m *Module) ListDemands(ctx context.Context, circleID, viewerID int64, limit int) ([]contract.Demand, error) {
	return m.svc.ListDemands(ctx, circleID, viewerID, limit)
}
func (m *Module) RenewDemand(ctx context.Context, demandID, userID int64) error {
	return m.svc.RenewDemand(ctx, demandID, userID)
}
func (m *Module) PublishFeed(ctx context.Context, circleID, userID int64, text string, imageFileIDs []int64, announcement bool) (int64, error) {
	return m.svc.PublishFeed(ctx, circleID, userID, text, imageFileIDs, announcement)
}
func (m *Module) ExpireDemands(ctx context.Context) error { return m.svc.ExpireDemands(ctx) }
func (m *Module) Mount(ctx context.Context, circleID, userID int64, itemRef string) (contract.Mount, bool, error) {
	return m.svc.Mount(ctx, circleID, userID, itemRef)
}
func (m *Module) ListMounts(ctx context.Context, circleID int64) ([]contract.Mount, error) {
	return m.svc.ListMounts(ctx, circleID)
}
func (m *Module) DecideMount(ctx context.Context, mountID, userID int64, approve bool) error {
	return m.svc.DecideMount(ctx, mountID, userID, approve)
}
func (m *Module) ToCube(ctx context.Context, circleID, demandID, cubeID, userID int64) (int64, error) {
	return m.svc.ToCube(ctx, circleID, demandID, cubeID, userID)
}

// ProviderFor trust.ReviewSubjectProvider（circle.demand）。
func (m *Module) ProviderFor() reviewProvider { return reviewProvider{m: m} }

type reviewProvider struct{ m *Module }

func (p reviewProvider) SubjectType() string { return "circle.demand" }
func (p reviewProvider) Extract(ctx context.Context, subjectID int64) (string, []string, error) {
	return p.m.svc.ReviewText(ctx, subjectID)
}
func (p reviewProvider) OwnerOf(ctx context.Context, subjectID int64) (int64, error) {
	return p.m.svc.DemandOwner(ctx, subjectID)
}
func (p reviewProvider) ApplyVerdict(_ context.Context, _ int64, _ bool, _ string) error { return nil }

// Resolve 实现 entityref.Resolver（circle 展示）。
func (m *Module) Resolve(ctx context.Context, ref entityref.Ref) (entityref.Info, error) {
	if ref.Type != "circle" {
		return entityref.Info{}, errors.New("circle: 不支持的类型")
	}
	b, err := m.svc.Brief(ctx, ref.ID)
	if err != nil {
		return entityref.Info{}, err
	}
	status := "active"
	if b.Status != contract.CircleActive {
		status = "inactive"
	}
	return entityref.Info{Title: b.Name, Status: status, Visibility: "platform"}, nil
}

// ---- HTTP ----

func (m *Module) handleCreate(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Name          string   `json:"name"`
		Intro         string   `json:"intro"`
		CoverFileID   int64    `json:"cover_file_id,string"`
		RegionCode    string   `json:"region_code"`
		IndustryCodes []string `json:"industry_codes"`
		Type          int16    `json:"type"`
		DurationDays  int      `json:"duration_days"`
		OrgID         int64    `json:"org_id,string"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	res, err := m.svc.Create(req.Context(), userIDFrom(req.Context()), contract.CreateInput{
		Name: body.Name, Intro: body.Intro, CoverFileID: body.CoverFileID, RegionCode: body.RegionCode,
		IndustryCodes: body.IndustryCodes, Type: body.Type, DurationDays: body.DurationDays, OrgID: body.OrgID,
	})
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, res)
}

func (m *Module) handleListMine(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.ListMine(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleDetail(w http.ResponseWriter, req *http.Request) {
	id := pathID(req, "id")
	viewer := int64(0)
	if token := bearer(req); token != "" {
		if uid, ver, err := m.deps.Identity.Authenticate(req.Context(), token); err == nil {
			if b, err := m.deps.Identity.UserBrief(req.Context(), uid); err == nil && b.AuthzVer == ver {
				viewer = uid
			}
		}
	}
	d, err := m.svc.Detail(req.Context(), viewer, id)
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, d)
}

func (m *Module) handleApply(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.Apply(req.Context(), pathID(req, "id"), userIDFrom(req.Context())); err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"applied": true})
}

func (m *Module) handleInvite(w http.ResponseWriter, req *http.Request) {
	code, expire, err := m.svc.InviteCode(req.Context(), pathID(req, "id"), userIDFrom(req.Context()))
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"code": code, "expire_at": expire})
}

func (m *Module) handleJoin(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Code == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	res, err := m.svc.JoinByInvite(req.Context(), userIDFrom(req.Context()), body.Code)
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, res)
}

func (m *Module) handleMembers(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.Members(req.Context(), pathID(req, "id"))
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleDissolve(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.Dissolve(req.Context(), pathID(req, "id"), userIDFrom(req.Context())); err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"dissolved": true})
}

func (m *Module) handleConvert(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.ConvertToPermanent(req.Context(), pathID(req, "id"), userIDFrom(req.Context())); err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"converted": true})
}

func (m *Module) handlePublishDemand(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Title         string   `json:"title"`
		Content       string   `json:"content"`
		DemandType    string   `json:"demand_type"`
		RegionCode    string   `json:"region_code"`
		IndustryCodes []string `json:"industry_codes"`
		Days          int      `json:"days"`
		Scope         int16    `json:"scope"`
		ContactPhone  string   `json:"contact_phone"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	d, err := m.svc.PublishDemand(req.Context(), pathID(req, "id"), userIDFrom(req.Context()), contract.DemandInput{
		Title: body.Title, Content: body.Content, DemandType: body.DemandType, RegionCode: body.RegionCode,
		IndustryCodes: body.IndustryCodes, Days: body.Days, Scope: body.Scope, ContactPhone: body.ContactPhone,
	})
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, d)
}

func (m *Module) handleListDemands(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	viewer := int64(0)
	if token := bearer(req); token != "" {
		if uid, ver, err := m.deps.Identity.Authenticate(req.Context(), token); err == nil {
			if b, err := m.deps.Identity.UserBrief(req.Context(), uid); err == nil && b.AuthzVer == ver {
				viewer = uid
			}
		}
	}
	items, err := m.svc.ListDemands(req.Context(), pathID(req, "id"), viewer, limit)
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleRenewDemand(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.RenewDemand(req.Context(), pathID(req, "did"), userIDFrom(req.Context())); err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"renewed": true})
}

func (m *Module) handlePublishFeed(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Text           string  `json:"text"`
		ImageFileIDs   []int64 `json:"image_file_ids"`
		IsAnnouncement bool    `json:"is_announcement"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	id, err := m.svc.PublishFeed(req.Context(), pathID(req, "id"), userIDFrom(req.Context()), body.Text, body.ImageFileIDs, body.IsAnnouncement)
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"id": strconv.FormatInt(id, 10)})
}

// ---- 挂载与到猫立方里办（§6.6/6.7）----

func (m *Module) handleMount(w http.ResponseWriter, req *http.Request) {
	var body struct {
		ItemRef string `json:"item_ref"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.ItemRef == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	mt, direct, err := m.svc.Mount(req.Context(), pathID(req, "id"), userIDFrom(req.Context()), body.ItemRef)
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"mount": mt, "direct": direct})
}

func (m *Module) handleListMounts(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.ListMounts(req.Context(), pathID(req, "id"))
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleDecideMount(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Approve bool `json:"approve"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	if err := m.svc.DecideMount(req.Context(), pathID(req, "mid"), userIDFrom(req.Context()), body.Approve); err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"approved": body.Approve})
}

func (m *Module) handleToCube(w http.ResponseWriter, req *http.Request) {
	var body struct {
		CubeID int64 `json:"cube_id,string"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.CubeID == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	projectID, err := m.svc.ToCube(req.Context(), pathID(req, "id"), pathID(req, "did"), body.CubeID, userIDFrom(req.Context()))
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"project_id": strconv.FormatInt(projectID, 10)})
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

func pathID(req *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(req.PathValue(name), 10, 64)
	return id
}

func writeCircleErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeErr(w, codeNotFound, err.Error())
	case errors.Is(err, domain.ErrNoPermission), errors.Is(err, domain.ErrNotRealname), errors.Is(err, domain.ErrEnded):
		writeErr(w, codeNoPermission, err.Error())
	case errors.Is(err, domain.ErrDuration):
		writeErr(w, codeDuration, err.Error())
	case errors.Is(err, domain.ErrCircleLimit):
		writeErr(w, codeCircleLimit, err.Error())
	case errors.Is(err, domain.ErrInviteInvalid):
		writeErr(w, codeInviteBad, err.Error())
	case errors.Is(err, domain.ErrPostQuota):
		writeErr(w, codePostQuota, err.Error())
	case errors.Is(err, domain.ErrAnnounceQuota):
		writeErr(w, codeAnnounceQuota, err.Error())
	case errors.Is(err, domain.ErrMountLimit):
		writeErr(w, codeMountLimit, err.Error())
	case errors.Is(err, domain.ErrInvalidInput):
		writeErr(w, codeInvalidInput, err.Error())
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeNoPermission:
		return http.StatusForbidden
	case codeNotFound:
		return http.StatusNotFound
	case codeCircleLimit, codePostQuota, codeAnnounceQuota:
		return http.StatusTooManyRequests
	case codeParamInvalid, codeNotRealname, codeDuration, codeInviteBad, codeInvalidInput:
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

type cubefacade = app.CubeFacade

var _ contract.Facade = (*Module)(nil)
var _ entityref.Resolver = (*Module)(nil)
var _ = fmt.Sprintf
