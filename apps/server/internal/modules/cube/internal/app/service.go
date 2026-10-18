// Package app cube 应用服务：空间创建、邀请制成员、转让、订阅与协作（事务/事件编排）。
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/kernel/settings"
	contentcontract "catcube/internal/modules/content/contract"
	"catcube/internal/modules/cube/contract"
	"catcube/internal/modules/cube/internal/domain"
	"catcube/internal/modules/cube/internal/infra"
	idcontract "catcube/internal/modules/identity/contract"
	msgcontract "catcube/internal/modules/msg/contract"
	tradecontract "catcube/internal/modules/trade/contract"
	"catcube/internal/shared/entityref"
	"catcube/pkg/ids"
)

// Repo 应用层仓储能力。
type Repo interface {
	CreateCube(ctx context.Context, q infra.Querier, c infra.Cube) error
	GetCube(ctx context.Context, id int64) (infra.Cube, error)
	ListCubesByUser(ctx context.Context, userID int64) ([]infra.Cube, error)
	AdminListCubes(ctx context.Context, q string, limit int) ([]infra.Cube, error)
	CountActiveCubes(ctx context.Context, userID int64) (int, error)
	AddMember(ctx context.Context, q infra.Querier, id, cubeID, userID int64, role int16, invitedBy int64) error
	GetMember(ctx context.Context, cubeID, userID int64) (infra.Member, bool, error)
	ListMembers(ctx context.Context, cubeID int64) ([]infra.Member, error)
	CountMembers(ctx context.Context, cubeID int64) (int, error)
	CountProjects(ctx context.Context, cubeID int64) (int, error)
	SetMemberRole(ctx context.Context, q infra.Querier, cubeID, userID int64, role int16) error
	RemoveMember(ctx context.Context, q infra.Querier, cubeID, userID int64, reason string) error
	UpdateLeader(ctx context.Context, q infra.Querier, cubeID, leaderID, honeyID int64, logJSON []byte) error
	SetHoney(ctx context.Context, q infra.Querier, cubeID, honeyID int64) error
	SetCubeStatus(ctx context.Context, q infra.Querier, cubeID int64, status int16) error
	SetPlan(ctx context.Context, cubeID int64, level int16, expireAt time.Time) error
	UpdateProfile(ctx context.Context, q infra.Querier, cubeID int64, name, region, chain, direction, resources, strength, cooperation string, industries []byte) error
	ListExpiredPlans(ctx context.Context, limit int) ([]int64, error)
	InsertInvite(ctx context.Context, inv infra.Invite) error
	GetInviteByCode(ctx context.Context, code string) (infra.Invite, error)
	IncrInviteUse(ctx context.Context, q infra.Querier, code string) error
	// 协作实体（§2）
	CreateProject(ctx context.Context, q infra.Querier, p infra.Project) error
	GetProject(ctx context.Context, id int64) (infra.Project, error)
	ListProjects(ctx context.Context, cubeID int64, typ, status int16) ([]infra.Project, error)
	SetProjectStatus(ctx context.Context, q infra.Querier, id int64, status int16) error
	CreateTask(ctx context.Context, q infra.Querier, t infra.Task) error
	GetTask(ctx context.Context, id int64) (infra.Task, error)
	ListTasks(ctx context.Context, cubeID, projectID int64) ([]infra.Task, error)
	UpdateTaskStatus(ctx context.Context, q infra.Querier, id int64, status int16, progress int16, rejectReason string) error
	ReopenTask(ctx context.Context, q infra.Querier, id int64, operatorID int64, logJSON []byte) error
	CreateReport(ctx context.Context, q infra.Querier, r infra.Report) error
	GetReport(ctx context.Context, id int64) (infra.Report, error)
	ListReports(ctx context.Context, cubeID, projectID int64) ([]infra.Report, error)
	SetReportStatus(ctx context.Context, q infra.Querier, id int64, status int16, rejectReason string, confirmOrg int64) error
	CountDuplicateReport(ctx context.Context, cubeID int64, customerName string) (int, error)
	CreateResearch(ctx context.Context, q infra.Querier, r infra.Research) error
	ListResearch(ctx context.Context, projectID int64) ([]infra.Research, error)
	UpdateProjectContentID(ctx context.Context, q infra.Querier, id, contentItemID int64) error
	CreateLinkReq(ctx context.Context, q infra.Querier, l infra.LinkReq) error
	GetLinkReq(ctx context.Context, id int64) (infra.LinkReq, error)
	ListLinkReqs(ctx context.Context, cubeID int64, status int16) ([]infra.LinkReq, error)
	SetLinkReqStatus(ctx context.Context, q infra.Querier, id int64, status int16) error
}

// Options 构造参数。
type Options struct {
	Repo      Repo
	Pool      *db.Pool
	Bus       *eventbus.Bus
	Policy    *policy.Engine
	Identity  idcontract.Facade
	Encryptor *sensitive.Encryptor
	Trade     tradecontract.Facade
	Settings  *settings.Registry
	Content   contentcontract.Facade
	Entities  *entityref.Registry
	Msg       msgcontract.Facade
}

// Service cube 应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	bus      *eventbus.Bus
	policy   *policy.Engine
	identity idcontract.Facade
	enc      *sensitive.Encryptor
	trade    tradecontract.Facade
	settings *settings.Registry
	content  contentcontract.Facade
	entities *entityref.Registry
	msg      msgcontract.Facade
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, policy: o.Policy,
		identity: o.Identity, enc: o.Encryptor, trade: o.Trade, settings: o.Settings,
		content: o.Content, entities: o.Entities, msg: o.Msg}
}

const inviteTTL = 7 * 24 * time.Hour

// Create 创建猫立方（已实名个人；创建者=猫王，必配猫蜜）。
func (s *Service) Create(ctx context.Context, userID int64, in contract.CreateInput) (contract.CubeBrief, error) {
	if err := domain.ValidateCreate(in.Name, in.HoneyUserID, userID); err != nil {
		return contract.CubeBrief{}, err
	}
	if err := s.requireRealname(ctx, userID); err != nil {
		return contract.CubeBrief{}, err
	}
	if err := s.requireRealname(ctx, in.HoneyUserID); err != nil {
		return contract.CubeBrief{}, err
	}
	tier := domain.Tier(1) // 新建立方默认基础档
	if err := s.checkCubeLimit(ctx, userID, tier); err != nil {
		return contract.CubeBrief{}, err
	}
	cubeID := int64(ids.New())
	c := infra.Cube{
		ID: cubeID, Name: in.Name, OrgID: in.OrgID, LeaderID: userID, HoneyID: in.HoneyUserID,
		RegionCode: in.RegionCode, Industry: in.IndustryCodes, ChainStage: in.ChainStage,
		Direction: in.Direction, Resources: in.Resources, Strength: in.Strength, Cooperation: in.Cooperation,
		PlanLevel: domain.DefaultTiers[0].Level,
	}
	err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.CreateCube(ctx, tx, c); err != nil {
			return err
		}
		if err := s.repo.AddMember(ctx, tx, int64(ids.New()), cubeID, userID, contract.RoleLeader, 0); err != nil {
			return err
		}
		if err := s.repo.AddMember(ctx, tx, int64(ids.New()), cubeID, in.HoneyUserID, contract.RoleHoney, userID); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewCubeCreated(ids.New().String(), contract.CubePayload{
			CubeID: cubeID, Name: in.Name, LeaderID: userID, UserID: in.HoneyUserID,
		}))
	})
	if err != nil {
		slog.Error("cube 创建失败", "err", err)
		return contract.CubeBrief{}, err
	}
	if s.msg != nil {
		if _, err := s.msg.EnsureGroup(ctx, "cube:"+strconv.FormatInt(cubeID, 10), in.Name+"·协作", userID); err != nil {
			slog.Error("cube 建群聊失败", "err", err)
		}
	}
	return s.brief(c, 2), nil
}

// Invite 生成邀请码（猫王/猫蜜）。
func (s *Service) Invite(ctx context.Context, cubeID, byUserID int64) (contract.InviteTicket, error) {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return contract.InviteTicket{}, err
	}
	if c.Status != contract.CubeActive {
		return contract.InviteTicket{}, domain.ErrNotFound
	}
	if err := s.requireRole(ctx, cubeID, byUserID, contract.RoleLeader, contract.RoleHoney); err != nil {
		return contract.InviteTicket{}, err
	}
	code := inviteCode()
	expire := time.Now().UTC().Add(inviteTTL)
	if err := s.repo.InsertInvite(ctx, infra.Invite{
		ID: int64(ids.New()), CubeID: cubeID, Code: code, CreatedBy: byUserID,
		ExpireAt: expire, NeedConfirm: true,
	}); err != nil {
		return contract.InviteTicket{}, err
	}
	return contract.InviteTicket{Code: code, ExpireAt: expire, NeedConfirm: true}, nil
}

// JoinByCode 凭邀请码加入（已实名、未被移除过）。
func (s *Service) JoinByCode(ctx context.Context, userID int64, code string) (contract.CubeBrief, error) {
	inv, err := s.repo.GetInviteByCode(ctx, code)
	if err != nil {
		return contract.CubeBrief{}, domain.ErrInviteInvalid
	}
	if inv.Status != 0 || time.Now().UTC().After(inv.ExpireAt) {
		return contract.CubeBrief{}, domain.ErrInviteInvalid
	}
	if err := s.requireRealname(ctx, userID); err != nil {
		return contract.CubeBrief{}, err
	}
	c, err := s.repo.GetCube(ctx, inv.CubeID)
	if err != nil {
		return contract.CubeBrief{}, err
	}
	if c.Status != contract.CubeActive {
		return contract.CubeBrief{}, domain.ErrNotFound
	}
	m, exists, err := s.repo.GetMember(ctx, inv.CubeID, userID)
	if err != nil {
		return contract.CubeBrief{}, err
	}
	if err := domain.CanJoin(m.Status, exists); err != nil {
		return contract.CubeBrief{}, err
	}
	tier := domain.Tier(c.PlanLevel)
	if err := s.checkCubeLimit(ctx, userID, tier); err != nil {
		return contract.CubeBrief{}, err
	}
	n, err := s.repo.CountMembers(ctx, inv.CubeID)
	if err != nil {
		return contract.CubeBrief{}, err
	}
	if tier.MemberLimit > 0 && n >= tier.MemberLimit {
		return contract.CubeBrief{}, domain.ErrMemberLimit
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.AddMember(ctx, tx, int64(ids.New()), inv.CubeID, userID, contract.RoleMember, inv.CreatedBy); err != nil {
			return err
		}
		if err := s.repo.IncrInviteUse(ctx, tx, code); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewMemberJoined(ids.New().String(), contract.MemberPayload{
			CubeID: inv.CubeID, UserID: userID, Role: contract.RoleMember, OperatorID: inv.CreatedBy,
		}))
	})
	if err != nil {
		return contract.CubeBrief{}, err
	}
	_ = s.identity.AuthzBump(ctx, userID)
	return s.brief(c, n+1), nil
}

// RemoveMember 移除成员（猫王/猫蜜；被移除者不可自行重进）。
func (s *Service) RemoveMember(ctx context.Context, cubeID, operatorID, targetID int64, reason string) error {
	if err := s.requireRole(ctx, cubeID, operatorID, contract.RoleLeader, contract.RoleHoney); err != nil {
		return err
	}
	if operatorID == targetID {
		return domain.ErrNoPermission
	}
	err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.RemoveMember(ctx, tx, cubeID, targetID, reason); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewMemberRemoved(ids.New().String(), contract.MemberPayload{
			CubeID: cubeID, UserID: targetID, OperatorID: operatorID, Reason: reason,
		}))
	})
	if err != nil {
		return err
	}
	return s.identity.AuthzBump(ctx, targetID)
}

// AdminRemoveMember 平台移除成员（不要求操作者是猫王/猫蜜；不可移除猫王）。
func (s *Service) AdminRemoveMember(ctx context.Context, cubeID, targetID int64, reason string) error {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return err
	}
	if targetID == c.LeaderID {
		return domain.ErrNoPermission
	}
	if targetID == c.HoneyID {
		return domain.ErrHoneyRequired // 需先调整猫蜜，再移除
	}
	if _, ok, err := s.repo.GetMember(ctx, cubeID, targetID); err != nil || !ok {
		return domain.ErrNoPermission
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.RemoveMember(ctx, tx, cubeID, targetID, reason); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewMemberRemoved(ids.New().String(), contract.MemberPayload{
			CubeID: cubeID, UserID: targetID, OperatorID: 0, Reason: reason,
		}))
	}); err != nil {
		return err
	}
	return s.identity.AuthzBump(ctx, targetID)
}

// SetHoneyByLeader 猫王指定猫蜜（需为在职成员，不能是猫王）。
func (s *Service) SetHoneyByLeader(ctx context.Context, cubeID, leaderID, userID int64) error {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return err
	}
	if c.LeaderID != leaderID {
		return domain.ErrNoPermission
	}
	if userID == c.LeaderID {
		return domain.ErrHoneyRequired
	}
	target, ok, err := s.repo.GetMember(ctx, cubeID, userID)
	if err != nil || !ok || target.Status != contract.MemberActive {
		return domain.ErrNoPermission
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if c.HoneyID != 0 && c.HoneyID != userID {
			if err := s.repo.SetMemberRole(ctx, tx, cubeID, c.HoneyID, contract.RoleMember); err != nil {
				return err
			}
		}
		if err := s.repo.SetHoney(ctx, tx, cubeID, userID); err != nil {
			return err
		}
		return s.repo.SetMemberRole(ctx, tx, cubeID, userID, contract.RoleHoney)
	}); err != nil {
		return err
	}
	if c.HoneyID != 0 && c.HoneyID != userID {
		_ = s.identity.AuthzBump(ctx, c.HoneyID)
	}
	return s.identity.AuthzBump(ctx, userID)
}

// AdminSetHoney 平台指定猫蜜（需为在职成员，不能是猫王）。
func (s *Service) AdminSetHoney(ctx context.Context, cubeID, userID int64) error {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return err
	}
	if userID == c.LeaderID {
		return domain.ErrHoneyRequired
	}
	target, ok, err := s.repo.GetMember(ctx, cubeID, userID)
	if err != nil || !ok || target.Status != contract.MemberActive {
		return domain.ErrNoPermission
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if c.HoneyID != 0 && c.HoneyID != userID {
			if err := s.repo.SetMemberRole(ctx, tx, cubeID, c.HoneyID, contract.RoleMember); err != nil {
				return err
			}
		}
		if err := s.repo.SetHoney(ctx, tx, cubeID, userID); err != nil {
			return err
		}
		return s.repo.SetMemberRole(ctx, tx, cubeID, userID, contract.RoleHoney)
	}); err != nil {
		return err
	}
	if c.HoneyID != 0 && c.HoneyID != userID {
		_ = s.identity.AuthzBump(ctx, c.HoneyID)
	}
	return s.identity.AuthzBump(ctx, userID)
}

// Leave 主动退出。
func (s *Service) Leave(ctx context.Context, cubeID, userID int64) error {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return err
	}
	if c.LeaderID == userID {
		return domain.ErrNoPermission // 猫王须先转让
	}
	m, exists, err := s.repo.GetMember(ctx, cubeID, userID)
	if err != nil || !exists || m.Status != contract.MemberActive {
		return domain.ErrNoPermission
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.RemoveMember(ctx, tx, cubeID, userID, "主动退出"); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewMemberRemoved(ids.New().String(), contract.MemberPayload{
			CubeID: cubeID, UserID: userID, OperatorID: userID, Reason: "leave",
		}))
	})
	if err != nil {
		return err
	}
	return s.identity.AuthzBump(ctx, userID)
}

// Transfer 转让猫王（仅猫王；目标须为成员）。
func (s *Service) Transfer(ctx context.Context, cubeID, fromUserID, toUserID int64) error {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return err
	}
	if c.LeaderID != fromUserID {
		return domain.ErrNoPermission
	}
	target, exists, err := s.repo.GetMember(ctx, cubeID, toUserID)
	if err != nil || !exists || target.Status != contract.MemberActive {
		return domain.ErrNoPermission
	}
	log, _ := json.Marshal([]map[string]any{{"from": fromUserID, "to": toUserID, "at": time.Now().UTC()}})
	honeyID := c.HoneyID
	if honeyID == toUserID {
		honeyID = fromUserID // 新猫王原为猫蜜时，旧猫王接任猫蜜
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.UpdateLeader(ctx, tx, cubeID, toUserID, honeyID, log); err != nil {
			return err
		}
		if err := s.repo.SetMemberRole(ctx, tx, cubeID, toUserID, contract.RoleLeader); err != nil {
			return err
		}
		fromRole := contract.RoleMember
		if honeyID == fromUserID {
			fromRole = contract.RoleHoney
		}
		if err := s.repo.SetMemberRole(ctx, tx, cubeID, fromUserID, fromRole); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	_ = s.identity.AuthzBump(ctx, fromUserID)
	_ = s.identity.AuthzBump(ctx, toUserID)
	return nil
}

// Dissolve 解散立方（猫王）。
func (s *Service) Dissolve(ctx context.Context, cubeID, userID int64) error {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return err
	}
	if c.LeaderID != userID {
		return domain.ErrNoPermission
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.SetCubeStatus(ctx, tx, cubeID, contract.CubeDissolved); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewCubeDissolved(ids.New().String(), contract.CubePayload{
			CubeID: cubeID, Name: c.Name, UserID: userID,
		}))
	})
	if err != nil {
		return err
	}
	return nil
}

// ExpireDue 订阅到期回落（SchedulerJob）：降回基础档。
func (s *Service) ExpireDue(ctx context.Context) error {
	list, err := s.repo.ListExpiredPlans(ctx, 200)
	if err != nil {
		return err
	}
	for _, cubeID := range list {
		if err := s.repo.SetPlan(ctx, cubeID, domain.DefaultTiers[0].Level, time.Time{}); err != nil {
			continue
		}
	}
	return nil
}

// ---- 查询 ----

// Brief 立方摘要。
func (s *Service) Brief(ctx context.Context, cubeID int64) (contract.CubeBrief, error) {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return contract.CubeBrief{}, err
	}
	n, err := s.repo.CountMembers(ctx, cubeID)
	if err != nil {
		return contract.CubeBrief{}, err
	}
	return s.brief(c, n), nil
}

// RoleOf 用户在立方中的角色（非成员返回 RoleNone）。
func (s *Service) RoleOf(ctx context.Context, cubeID, userID int64) (int16, error) {
	m, exists, err := s.repo.GetMember(ctx, cubeID, userID)
	if err != nil {
		return contract.RoleNone, err
	}
	if !exists || m.Status == contract.MemberRemoved {
		return contract.RoleNone, nil
	}
	return m.Role, nil
}

// ListMine 我参与的立方。
func (s *Service) ListMine(ctx context.Context, userID int64) ([]contract.CubeBrief, error) {
	cs, err := s.repo.ListCubesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.CubeBrief, 0, len(cs))
	for _, c := range cs {
		n, err := s.repo.CountMembers(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, s.brief(c, n))
	}
	return out, nil
}

// Detail 立方详情（含我的角色）。
func (s *Service) Detail(ctx context.Context, viewerID, cubeID int64) (contract.CubeDetail, error) {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return contract.CubeDetail{}, err
	}
	n, err := s.repo.CountMembers(ctx, cubeID)
	if err != nil {
		return contract.CubeDetail{}, err
	}
	role, err := s.RoleOf(ctx, cubeID, viewerID)
	if err != nil {
		return contract.CubeDetail{}, err
	}
	return contract.CubeDetail{
		CubeBrief: s.brief(c, n), OrgID: c.OrgID, IndustryCodes: c.Industry,
		ChainStage: c.ChainStage, Direction: c.Direction, Resources: c.Resources,
		Strength: c.Strength, Cooperation: c.Cooperation, MyRole: role,
	}, nil
}

// Members 成员列表（含昵称）。
func (s *Service) Members(ctx context.Context, cubeID int64) ([]contract.Member, error) {
	ms, err := s.repo.ListMembers(ctx, cubeID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Member, 0, len(ms))
	for _, m := range ms {
		item := contract.Member{UserID: m.UserID, Role: m.Role, Status: m.Status, JoinedAt: m.JoinedAt}
		if s.identity != nil {
			if b, err := s.identity.UserBrief(ctx, m.UserID); err == nil {
				item.Nickname = b.Nickname
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// UpdateProfile 编辑关于我们（猫王/猫蜜）。
func (s *Service) UpdateProfile(ctx context.Context, cubeID, userID int64, in contract.ProfileInput) error {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return err
	}
	if c.Status != contract.CubeActive {
		return domain.ErrNotFound
	}
	ok, err := s.canManage(ctx, cubeID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNoPermission
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = c.Name
	}
	ind, _ := json.Marshal(in.IndustryCodes)
	return s.repo.UpdateProfile(ctx, s.pool, cubeID, name, strings.TrimSpace(in.RegionCode),
		strings.TrimSpace(in.ChainStage), strings.TrimSpace(in.Direction), strings.TrimSpace(in.Resources),
		strings.TrimSpace(in.Strength), strings.TrimSpace(in.Cooperation), ind)
}

// EnsureCubeChat 进入猫立方工作群（仅成员）。
func (s *Service) EnsureCubeChat(ctx context.Context, cubeID, userID int64) (msgcontract.Session, error) {
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return msgcontract.Session{}, err
	}
	if c.Status != contract.CubeActive {
		return msgcontract.Session{}, domain.ErrNotFound
	}
	role, err := s.RoleOf(ctx, cubeID, userID)
	if err != nil {
		return msgcontract.Session{}, err
	}
	if role == contract.RoleNone {
		return msgcontract.Session{}, domain.ErrNoPermission
	}
	if s.msg == nil {
		return msgcontract.Session{}, domain.ErrInvalidInput
	}
	return s.msg.EnsureGroup(ctx, "cube:"+strconv.FormatInt(cubeID, 10), c.Name+"·协作", userID)
}

// EnsureProjectChat 进入项目群（仅立方成员）。
func (s *Service) EnsureProjectChat(ctx context.Context, cubeID, projectID, userID int64) (msgcontract.Session, error) {
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return msgcontract.Session{}, err
	}
	if p.CubeID != cubeID {
		return msgcontract.Session{}, domain.ErrNotFound
	}
	role, err := s.RoleOf(ctx, cubeID, userID)
	if err != nil {
		return msgcontract.Session{}, err
	}
	if role == contract.RoleNone {
		return msgcontract.Session{}, domain.ErrNoPermission
	}
	if s.msg == nil {
		return msgcontract.Session{}, domain.ErrInvalidInput
	}
	return s.msg.EnsureGroup(ctx, "cube.project:"+strconv.FormatInt(projectID, 10), p.Name, userID)
}

// AdminList 后台立方列表（名称模糊）。
func (s *Service) AdminList(ctx context.Context, q string, limit int) ([]contract.CubeBrief, error) {
	cs, err := s.repo.AdminListCubes(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	out := make([]contract.CubeBrief, 0, len(cs))
	for _, c := range cs {
		n, _ := s.repo.CountMembers(ctx, c.ID)
		b := s.brief(c, n)
		b.ProjectCount, _ = s.repo.CountProjects(ctx, c.ID)
		created := c.CreatedAt
		b.CreatedAt = &created
		out = append(out, b)
	}
	return out, nil
}

// HasActiveMembership 用户是否属于任一正常立方（circle 前提校验）。
func (s *Service) HasActiveMembership(ctx context.Context, userID int64) (bool, error) {
	n, err := s.repo.CountActiveCubes(ctx, userID)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ---- 内部 ----

func (s *Service) brief(c infra.Cube, memberCount int) contract.CubeBrief {
	return contract.CubeBrief{
		ID: c.ID, Name: c.Name, LeaderID: c.LeaderID, HoneyID: c.HoneyID,
		PlanLevel: c.PlanLevel, PlanExpireAt: c.PlanExpireAt, Status: c.Status,
		MemberCount: memberCount, RegionCode: c.RegionCode,
	}
}

func (s *Service) requireRealname(ctx context.Context, userID int64) error {
	if s.policy == nil {
		return nil
	}
	ok, err := s.policy.Has(ctx, s.policy.NewActor(userID, 1), "realname")
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotRealname
	}
	return nil
}

func (s *Service) requireRole(ctx context.Context, cubeID, userID int64, roles ...int16) error {
	role, err := s.RoleOf(ctx, cubeID, userID)
	if err != nil {
		return err
	}
	for _, r := range roles {
		if role == r {
			return nil
		}
	}
	return domain.ErrNoPermission
}

func (s *Service) checkCubeLimit(ctx context.Context, userID int64, tier domain.PlanTier) error {
	if tier.CubeLimit == 0 {
		return nil
	}
	n, err := s.repo.CountActiveCubes(ctx, userID)
	if err != nil {
		return err
	}
	if n >= tier.CubeLimit {
		return domain.ErrCubeLimit
	}
	return nil
}

func (s *Service) publish(ctx context.Context, tx db.Tx, e eventbus.Event) error {
	if s.bus == nil {
		return nil
	}
	return s.bus.Publish(ctx, tx, e)
}

func inviteCode() string {
	return "C" + strconv.FormatInt(int64(ids.New()), 36)
}
