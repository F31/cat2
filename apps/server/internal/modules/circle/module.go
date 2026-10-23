// Package circle R4 圈子模块：生命周期、加入四路、圈内发布与挂载、到猫立方里办。
package circle

import (
	"context"
	"encoding/base64"
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
	"catcube/internal/kernel/rbac"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/modules/circle/contract"
	"catcube/internal/modules/circle/internal/app"
	"catcube/internal/modules/circle/internal/domain"
	"catcube/internal/modules/circle/internal/infra"
	contentcontract "catcube/internal/modules/content/contract"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/ports"
	"catcube/internal/shared/entityref"
)

// Deps 装配依赖。
type Deps struct {
	// Audit 代操作审计写入器（可空）
	Audit     *audit.Recorder
	Pool      *db.Pool
	Bus       *eventbus.Bus
	Identity  idcontract.Facade
	Policy    *policy.Engine
	Encryptor *sensitive.Encryptor
	Entities  *entityref.Registry
	Content   contentcontract.Facade
	Cube      cubefacade
	Coder     ports.MiniProgramCoder
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
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/discover", Handler: m.handleDiscover},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}", Handler: m.handleDetail},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/apply", Handler: m.auth(m.handleApply)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/invite", Handler: m.auth(m.handleInvite)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/invite/reset", Handler: m.auth(m.handleResetInvite)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}/card", Handler: m.auth(m.handleCard)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/join", Handler: m.auth(m.handleJoin)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}/members", Handler: m.auth(m.handleMembers)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}/applications", Handler: m.auth(m.handleApplications)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/applications/{uid}/decide", Handler: m.auth(m.handleDecideApplication)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}/feeds", Handler: m.auth(m.handleListFeeds)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/dissolve", Handler: m.auth(m.handleDissolve)},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/circles", Handler: m.auth(m.handleAdminList), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/circles/stats", Handler: m.auth(m.handleAdminStats), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/circles/{id}", Handler: m.auth(m.handleAdminDetail), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/circles/{id}/demands/{did}/remove", Handler: m.auth(m.handleAdminRemoveDemand), Admin: true, Perm: rbac.PermModeration},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/circles/{id}/feeds/{fid}/remove", Handler: m.auth(m.handleAdminRemoveFeed), Admin: true, Perm: rbac.PermModeration},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/circles/{id}/mounts/{mid}/remove", Handler: m.auth(m.handleAdminRemoveMount), Admin: true, Perm: rbac.PermModeration},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/circles/{id}/dissolve", Handler: m.auth(m.handleAdminDissolve), Admin: true, Perm: rbac.PermModeration},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/circles/batch", Handler: m.auth(m.handleBatchStatus), Admin: true, Perm: rbac.PermModeration},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/convert", Handler: m.auth(m.handleConvert)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/demands", Handler: m.auth(m.handlePublishDemand)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}/demands", Handler: m.auth(m.handleListDemands)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/demands/{did}/renew", Handler: m.auth(m.handleRenewDemand)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/feeds", Handler: m.auth(m.handlePublishFeed)},
		module.Route{Method: http.MethodDelete, Path: "/api/mp/v1/circles/{id}/feeds/{fid}", Handler: m.auth(m.handleDeleteFeed)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/circles/{id}/mounts", Handler: m.auth(m.handleMount)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/circles/{id}/mounts", Handler: m.auth(m.handleListMounts)},
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
		audit.Point{Action: "circle:admin:dissolve", Path: "/api/admin/v1/circles/{id}/dissolve", Required: true, LogFailed: true, Module: "circle"},
		audit.Point{Action: "circle:admin:ban", Path: "/api/admin/v1/circles/batch", Required: true, LogFailed: true, Module: "circle"},
		audit.Point{Action: "circle:admin:archive", Path: "/api/admin/v1/circles/batch", Required: true, LogFailed: true, Module: "circle"},
		audit.Point{Action: "circle:admin:unban", Path: "/api/admin/v1/circles/batch", Required: true, LogFailed: true, Module: "circle"},
		audit.Point{Action: "circle:admin:restore", Path: "/api/admin/v1/circles/batch", Required: true, LogFailed: true, Module: "circle"},
		audit.Point{Action: "circle:convert", Path: "/api/mp/v1/circles/{id}/convert", Required: true, LogFailed: false, Module: "circle"},
		audit.Point{Action: "circle:demand:remove", Path: "/api/admin/v1/circles/{id}/demands/{did}/remove", Required: true, LogFailed: true, Module: "circle"},
		audit.Point{Action: "circle:feed:remove", Path: "/api/admin/v1/circles/{id}/feeds/{fid}/remove", Required: true, LogFailed: true, Module: "circle"},
		audit.Point{Action: "circle:mount:remove", Path: "/api/admin/v1/circles/{id}/mounts/{mid}/remove", Required: true, LogFailed: true, Module: "circle"},
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
func (m *Module) ListMounts(ctx context.Context, circleID, viewerID int64) ([]contract.Mount, error) {
	return m.svc.ListMounts(ctx, circleID, viewerID)
}
func (m *Module) DecideMount(ctx context.Context, mountID, userID int64, approve bool) error {
	return m.svc.DecideMount(ctx, mountID, userID, approve)
}
func (m *Module) ToCube(ctx context.Context, circleID, demandID, cubeID, userID int64) (int64, error) {
	return m.svc.ToCube(ctx, circleID, demandID, cubeID, userID)
}

func (m *Module) ContentMounted(ctx context.Context, circleID int64, itemRef string) (bool, error) {
	return m.svc.ContentMounted(ctx, circleID, itemRef)
}
func (m *Module) Discover(ctx context.Context, viewerID int64, f contract.DiscoverFilter) ([]contract.CircleDiscoverItem, string, error) {
	return m.svc.Discover(ctx, viewerID, f)
}
func (m *Module) MemberItems(ctx context.Context, circleID, viewerID int64) ([]contract.CircleMember, error) {
	return m.svc.MemberItems(ctx, circleID, viewerID)
}
func (m *Module) ListFeeds(ctx context.Context, circleID, viewerID int64, limit int) ([]contract.CircleFeed, error) {
	return m.svc.ListFeeds(ctx, circleID, viewerID, limit)
}
func (m *Module) DeleteFeed(ctx context.Context, circleID, feedID, userID int64) error {
	return m.svc.DeleteFeed(ctx, circleID, feedID, userID)
}
func (m *Module) ListApplications(ctx context.Context, circleID, ownerID int64, status int16) ([]contract.CircleApplication, error) {
	return m.svc.ListApplications(ctx, circleID, ownerID, status)
}
func (m *Module) DecideApplication(ctx context.Context, circleID, ownerID, applicantID int64, approve bool, reason string) error {
	return m.svc.DecideApplication(ctx, circleID, ownerID, applicantID, approve, reason)
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

// handleDiscover 圈子发现（公开；登录后带加入/申请状态）。
func (m *Module) handleDiscover(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("cursor"))
	typ, _ := strconv.Atoi(q.Get("type"))
	items, next, err := m.svc.Discover(req.Context(), m.optionalUserID(req), contract.DiscoverFilter{
		Q: q.Get("q"), Region: q.Get("region"), Type: int16(typ), Sort: q.Get("sort"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	var cursor any
	if next != "" {
		cursor = next
	}
	writeData(w, map[string]any{"items": items, "next_cursor": cursor})
}

func (m *Module) handleDetail(w http.ResponseWriter, req *http.Request) {
	id := pathID(req, "id")
	viewer := m.optionalUserID(req)
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
	items, err := m.svc.MemberItems(req.Context(), pathID(req, "id"), userIDFrom(req.Context()))
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

// handleApplications 圈主查看入圈申请（status=0/1/2；缺省待审）。
func (m *Module) handleApplications(w http.ResponseWriter, req *http.Request) {
	status := contract.ApplyPending
	if v, err := strconv.Atoi(req.URL.Query().Get("status")); err == nil {
		status = int16(v)
	}
	items, err := m.svc.ListApplications(req.Context(), pathID(req, "id"), userIDFrom(req.Context()), status)
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

// handleDecideApplication 圈主通过/拒绝入圈申请。
func (m *Module) handleDecideApplication(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	err := m.svc.DecideApplication(req.Context(), pathID(req, "id"), userIDFrom(req.Context()), pathID(req, "uid"), body.Approve, body.Reason)
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"approved": body.Approve})
}

// handleListFeeds 圈内动态/公告（成员可见）。
func (m *Module) handleListFeeds(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	items, err := m.svc.ListFeeds(req.Context(), pathID(req, "id"), userIDFrom(req.Context()), limit)
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

// handleAdminDissolve 平台代操作：解散圈子（before/after 快照入审计）。
// handleAdminDetail 平台圈子详情：关于、成员、动态、需求、挂载（最近 50 条）。
func (m *Module) handleAdminDetail(w http.ResponseWriter, req *http.Request) {
	id := pathID(req, "id")
	var c map[string]any
	var name, intro, region string
	var typ, status int16
	var ownerID int64
	var members int
	var created time.Time
	var endAt *time.Time
	err := m.deps.Pool.QueryRow(req.Context(), `
		SELECT name, COALESCE(intro,''), type, status, owner_user_id, member_count, COALESCE(region_code,''), created_at, end_at
		FROM cir_circle WHERE id=$1`, id).
		Scan(&name, &intro, &typ, &status, &ownerID, &members, &region, &created, &endAt)
	if err != nil {
		writeErr(w, 140001, "圈子不存在")
		return
	}
	ownerName := ""
	if b, err := m.deps.Identity.UserBrief(req.Context(), ownerID); err == nil {
		ownerName = b.Nickname
	}
	c = map[string]any{"id": strconv.FormatInt(id, 10), "name": name, "intro": intro, "type": typ,
		"status": status, "owner_user_id": strconv.FormatInt(ownerID, 10), "member_count": members,
		"region_code": region, "created_at": created.Unix()}
	if endAt != nil {
		c["end_at"] = endAt.Unix()
	}

	memberRows, _ := m.deps.Pool.Query(req.Context(), `
		SELECT user_ref, joined_via, joined_at FROM cir_member WHERE circle_id=$1 AND status=0 ORDER BY joined_at`, id)
	memberItems := []map[string]any{}
	if memberRows != nil {
		defer memberRows.Close()
		for memberRows.Next() {
			var uid int64
			var via int16
			var joined time.Time
			if memberRows.Scan(&uid, &via, &joined) == nil {
				nick := ""
				if b, err := m.deps.Identity.UserBrief(req.Context(), uid); err == nil {
					nick = b.Nickname
				}
				memberItems = append(memberItems, map[string]any{"user_id": strconv.FormatInt(uid, 10),
					"nickname": nick, "joined_via": via, "joined_at": joined.Unix()})
			}
		}
	}
	demands := m.adminDemands(req.Context(), id)
	feeds := m.adminFeeds(req.Context(), id)
	mounts := m.adminMounts(req.Context(), id)
	writeData(w, map[string]any{"circle": c, "owner_name": ownerName, "members": memberItems,
		"demands": demands, "feeds": feeds, "mounts": mounts})
}

func (m *Module) adminDemands(ctx context.Context, circleID int64) []map[string]any {
	rows, err := m.deps.Pool.Query(ctx, `
		SELECT id, author_id, title, content, status, created_at FROM cir_demand
		WHERE circle_id=$1 AND status<>2 ORDER BY created_at DESC LIMIT 50`, circleID)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, author int64
		var title, content string
		var status int16
		var created time.Time
		if rows.Scan(&id, &author, &title, &content, &status, &created) == nil {
			nick := ""
			if b, err := m.deps.Identity.UserBrief(ctx, author); err == nil {
				nick = b.Nickname
			}
			out = append(out, map[string]any{"id": strconv.FormatInt(id, 10), "author_id": strconv.FormatInt(author, 10),
				"author_name": nick, "title": title, "content": content, "status": status, "created_at": created.Unix()})
		}
	}
	return out
}

func (m *Module) adminFeeds(ctx context.Context, circleID int64) []map[string]any {
	rows, err := m.deps.Pool.Query(ctx, `
		SELECT id, author_id, COALESCE(text,''), is_announcement, status, created_at FROM cir_feed
		WHERE circle_id=$1 AND status<>2 ORDER BY created_at DESC LIMIT 50`, circleID)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, author int64
		var text string
		var announce bool
		var status int16
		var created time.Time
		if rows.Scan(&id, &author, &text, &announce, &status, &created) == nil {
			nick := ""
			if b, err := m.deps.Identity.UserBrief(ctx, author); err == nil {
				nick = b.Nickname
			}
			out = append(out, map[string]any{"id": strconv.FormatInt(id, 10), "author_id": strconv.FormatInt(author, 10),
				"author_name": nick, "text": text, "is_announcement": announce, "status": status, "created_at": created.Unix()})
		}
	}
	return out
}

func (m *Module) adminMounts(ctx context.Context, circleID int64) []map[string]any {
	rows, err := m.deps.Pool.Query(ctx, `
		SELECT id, item_ref, mount_type, approval_status, status, created_at FROM cir_mount
		WHERE circle_id=$1 AND status=0 ORDER BY created_at DESC LIMIT 50`, circleID)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var ref string
		var mountType, approval, status int16
		var created time.Time
		if rows.Scan(&id, &ref, &mountType, &approval, &status, &created) == nil {
			out = append(out, map[string]any{"id": strconv.FormatInt(id, 10), "item_ref": ref,
				"mount_type": mountType, "approval_status": approval, "status": status, "created_at": created.Unix()})
		}
	}
	return out
}

// handleAdminStats 数据概览：新增圈数（常设/临时）、周期活跃圈、成员趋势。
func (m *Module) handleAdminStats(w http.ResponseWriter, req *http.Request) {
	days, _ := strconv.Atoi(req.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 30
	}
	var total, permanent, temporary int
	_ = m.deps.Pool.QueryRow(req.Context(), `
		SELECT count(1), count(1) FILTER (WHERE type=1), count(1) FILTER (WHERE type=2)
		FROM cir_circle WHERE created_at >= now() - make_interval(days => $1)`, days).Scan(&total, &permanent, &temporary)
	var active, membersTotal int
	_ = m.deps.Pool.QueryRow(req.Context(), `
		SELECT count(DISTINCT circle_id) FROM (
			SELECT circle_id FROM cir_demand WHERE created_at >= now() - make_interval(days => $1)
			UNION SELECT circle_id FROM cir_feed WHERE created_at >= now() - make_interval(days => $1)
			UNION SELECT circle_id FROM cir_member WHERE joined_at >= now() - make_interval(days => $1)
		) x`, days).Scan(&active)
	_ = m.deps.Pool.QueryRow(req.Context(), `SELECT count(1) FROM cir_member WHERE status=0`).Scan(&membersTotal)
	var demandTotal, demandValid int
	_ = m.deps.Pool.QueryRow(req.Context(), `
		SELECT count(1), count(1) FILTER (WHERE expire_at > now() AND status <> 2)
		FROM cir_demand WHERE created_at >= now() - make_interval(days => $1)`, days).Scan(&demandTotal, &demandValid)
	rate := 0.0
	if demandTotal > 0 {
		rate = float64(demandValid) * 100 / float64(demandTotal)
	}
	trend := []map[string]any{}
	rows, err := m.deps.Pool.Query(req.Context(), `
		SELECT to_char(date_trunc('day', joined_at), 'YYYY-MM-DD'), count(1) FROM cir_member
		WHERE status=0 AND joined_at >= now() - make_interval(days => $1)
		GROUP BY 1 ORDER BY 1`, days)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var d string
			var n int
			if rows.Scan(&d, &n) == nil {
				trend = append(trend, map[string]any{"date": d, "new_members": n})
			}
		}
	}
	writeData(w, map[string]any{"days": days, "new_total": total, "new_permanent": permanent,
		"new_temporary": temporary, "active_circles": active, "members_total": membersTotal,
		"demands_total": demandTotal, "demand_valid_rate": rate, "member_trend": trend})
}

func (m *Module) handleAdminRemoveDemand(w http.ResponseWriter, req *http.Request) {
	m.removeCircleContent(w, req, "did", "circle:demand:remove", "cir_demand")
}
func (m *Module) handleAdminRemoveFeed(w http.ResponseWriter, req *http.Request) {
	m.removeCircleContent(w, req, "fid", "circle:feed:remove", "cir_feed")
}
func (m *Module) handleAdminRemoveMount(w http.ResponseWriter, req *http.Request) {
	m.removeCircleContent(w, req, "mid", "circle:mount:remove", "cir_mount")
}

// removeCircleContent 平台内容处置：软删（status=2）并写审计。
func (m *Module) removeCircleContent(w http.ResponseWriter, req *http.Request, param, action, table string) {
	circleID := pathID(req, "id")
	itemID := pathID(req, param)
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	if strings.TrimSpace(body.Reason) == "" {
		writeErr(w, codeParamInvalid, "必须填写原因")
		return
	}
	tag, err := m.deps.Pool.Exec(req.Context(), `UPDATE `+table+` SET status=2 WHERE id=$1 AND circle_id=$2`, itemID, circleID)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 140001, "内容不存在")
		return
	}
	if table == "cir_feed" {
		_, _ = m.deps.Pool.Exec(req.Context(), `UPDATE cir_circle SET feed_count=GREATEST(feed_count-1,0) WHERE id=$1`, circleID)
	}
	if m.deps.Audit != nil {
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin", Action: action,
			TargetRef: fmt.Sprintf(`{"type":"%s","id":"%d"}`, table, itemID),
			Detail:    map[string]any{"circle_id": strconv.FormatInt(circleID, 10), "reason": body.Reason},
			Result:    audit.ResultOK, IP: audit.ClientIP(req), RequestID: audit.RequestIDOf(req.Context()),
		})
	}
	writeData(w, map[string]any{"removed": true})
}

// handleAdminList 平台圈子列表（名称模糊检索；id 一律以字符串返回）。
func (m *Module) handleAdminList(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := strings.TrimSpace(req.URL.Query().Get("q"))
	status := -1
	if v, err := strconv.Atoi(req.URL.Query().Get("status")); err == nil {
		status = v
	}
	rows, err := m.deps.Pool.Query(req.Context(), `
		SELECT id, name, type, status, owner_user_id, member_count, COALESCE(region_code,''), created_at, prev_status
		FROM cir_circle
		WHERE ($1 = '' OR name ILIKE '%' || $1 || '%') AND ($3 < 0 OR status=$3)
		ORDER BY created_at DESC LIMIT $2`, q, limit, status)
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id, owner int64
		var name, region string
		var typ, cstatus int16
		var prev *int16
		var members int
		var created time.Time
		if err := rows.Scan(&id, &name, &typ, &cstatus, &owner, &members, &region, &created, &prev); err != nil {
			writeErr(w, 9, "服务错误")
			return
		}
		items = append(items, map[string]any{
			"id": strconv.FormatInt(id, 10), "name": name, "type": typ, "status": cstatus,
			"prev_status": prev, "owner_user_id": strconv.FormatInt(owner, 10), "member_count": members,
			"region_code": region, "created_at": created.Unix(),
		})
	}
	writeData(w, map[string]any{"items": items})
}

// handleBatchStatus 平台批量封禁/归档/解封/恢复（≤100；必填原因；逐圈审计与结果）。
func (m *Module) handleBatchStatus(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Action string   `json:"action"`
		IDs    []string `json:"ids"`
		Reason string   `json:"reason"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil || len(body.IDs) == 0 || len(body.IDs) > 100 || strings.TrimSpace(body.Reason) == "" {
		writeErr(w, codeParamInvalid, "参数错误（1-100 个圈子且必须给原因）")
		return
	}
	action := body.Action
	switch action {
	case "ban", "archive", "unban", "restore":
	default:
		writeErr(w, codeParamInvalid, "不支持的动作")
		return
	}
	type item struct {
		ID     string `json:"id"`
		OK     bool   `json:"ok"`
		Before int16  `json:"before,omitempty"`
		After  int16  `json:"after,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	items := make([]item, 0, len(body.IDs))
	succeeded, failed := 0, 0
	for _, raw := range body.IDs {
		id, _ := strconv.ParseInt(raw, 10, 64)
		if id == 0 {
			items = append(items, item{ID: raw, Error: "非法圈子ID"})
			failed++
			continue
		}
		before, after, err := m.svc.AdminSetStatus(req.Context(), id, action)
		if err != nil {
			items = append(items, item{ID: raw, Error: err.Error()})
			failed++
			m.auditCircleBatch(req, action, id, before, 0, body.Reason, false)
			continue
		}
		m.auditCircleBatch(req, action, id, before, after, body.Reason, true)
		items = append(items, item{ID: raw, OK: true, Before: before, After: after})
		succeeded++
	}
	writeData(w, map[string]any{"items": items, "succeeded": succeeded, "failed": failed})
}

// auditCircleBatch 批量处置逐圈审计（前后状态 + 批量原因）。
func (m *Module) auditCircleBatch(req *http.Request, action string, id int64, before, after int16, reason string, ok bool) {
	if m.deps.Audit == nil {
		return
	}
	res := audit.ResultOK
	if !ok {
		res = audit.ResultFailed
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin",
		Action: "circle:admin:" + action, TargetRef: fmt.Sprintf(`{"id":"%d","type":"circle"}`, id),
		Detail: audit.Change{Before: map[string]any{"status": before}, After: map[string]any{"status": after}}.Snapshot(reason),
		Result: res, IP: audit.ClientIP(req), RequestID: audit.RequestIDOf(req.Context()),
	})
}

// handleResetInvite 圈主重置邀请（旧链接/二维码立即失效）。
func (m *Module) handleResetInvite(w http.ResponseWriter, req *http.Request) {
	n, err := m.svc.ResetInvites(req.Context(), pathID(req, "id"), userIDFrom(req.Context()))
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"invalidated": n})
}

// handleCard 圈名片数据：圈信息 + 小程序码（前端 Canvas 负责排版与保存）。
func (m *Module) handleCard(w http.ResponseWriter, req *http.Request) {
	id := pathID(req, "id")
	d, code, err := m.svc.CardContext(req.Context(), id, userIDFrom(req.Context()))
	if err != nil {
		writeCircleErr(w, err)
		return
	}
	ownerName := ""
	if b, err := m.deps.Identity.UserBrief(req.Context(), d.OwnerUserID); err == nil {
		ownerName = b.Nickname
	}
	if ownerName == "" {
		ownerName = "圈主"
	}
	var qr []byte
	if m.deps.Coder != nil {
		qr, err = m.deps.Coder.MiniProgramCode(req.Context(), "pages/circle/index", "c="+code)
		if err != nil {
			writeErr(w, codeInternal, "小程序码生成失败")
			return
		}
	}
	writeData(w, map[string]any{
		"circle_id": d.ID, "name": d.Name, "intro": d.Intro, "member_count": d.MemberCount,
		"owner_name": ownerName, "invite_code": code,
		"qr_png_base64": base64.StdEncoding.EncodeToString(qr),
	})
}

func (m *Module) handleAdminDissolve(w http.ResponseWriter, req *http.Request) {
	id := pathID(req, "id")
	var body struct {
		Reason string `json:"reason"`
	}
	if id == 0 || json.NewDecoder(req.Body).Decode(&body) != nil || body.Reason == "" {
		writeErr(w, 1, "参数错误（必须给原因）")
		return
	}
	var before int16
	if err := m.deps.Pool.QueryRow(req.Context(), `SELECT status FROM cir_circle WHERE id=$1`, id).Scan(&before); err != nil {
		writeErr(w, 4, "圈子不存在")
		return
	}
	if err := m.svc.AdminDissolve(req.Context(), id); err != nil {
		writeCircleErr(w, err)
		return
	}
	if m.deps.Audit != nil {
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin", Action: "circle:admin:dissolve",
			TargetRef: fmt.Sprintf(`{"id":"%d","type":"circle"}`, id),
			Detail:    audit.Change{Before: map[string]any{"status": before}, After: map[string]any{"status": 2}}.Snapshot(body.Reason),
			Result:    audit.ResultOK, IP: audit.ClientIP(req),
		})
	}
	writeData(w, map[string]any{"dissolved": true})
}

func (m *Module) handleConvert(w http.ResponseWriter, req *http.Request) {
	id := pathID(req, "id")
	uid := userIDFrom(req.Context())
	if err := m.svc.ConvertToPermanent(req.Context(), id, uid); err != nil {
		writeCircleErr(w, err)
		return
	}
	if m.deps.Audit != nil {
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: uid, ActorType: audit.ActorUser, Channel: "mp", Action: "circle:convert",
			TargetRef: fmt.Sprintf(`{"type":"circle","id":"%d"}`, id),
			Detail:    audit.Change{Before: map[string]any{"type": "temporary"}, After: map[string]any{"type": "permanent"}}.Snapshot("转常设圈"),
			Result:    audit.ResultOK, IP: audit.ClientIP(req), RequestID: audit.RequestIDOf(req.Context()),
		})
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
	items, err := m.svc.ListDemands(req.Context(), pathID(req, "id"), userIDFrom(req.Context()), limit)
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

func (m *Module) handleDeleteFeed(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.DeleteFeed(req.Context(), pathID(req, "id"), pathID(req, "fid"), userIDFrom(req.Context())); err != nil {
		writeCircleErr(w, err)
		return
	}
	writeData(w, map[string]any{"deleted": true})
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
	items, err := m.svc.ListMounts(req.Context(), pathID(req, "id"), userIDFrom(req.Context()))
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

func (m *Module) optionalUserID(req *http.Request) int64 {
	token := bearer(req)
	if token == "" {
		return 0
	}
	uid, ver, err := m.deps.Identity.Authenticate(req.Context(), token)
	if err != nil {
		return 0
	}
	b, err := m.deps.Identity.UserBrief(req.Context(), uid)
	if err != nil || b.AuthzVer != ver || (b.Status != 0 && b.Status != 1) {
		return 0
	}
	return uid
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
