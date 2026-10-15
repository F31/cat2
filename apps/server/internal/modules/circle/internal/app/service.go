// Package app circle 应用服务：圈子生命周期、加入四路、圈内发布与挂载。
package app

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/modules/circle/contract"
	"catcube/internal/modules/circle/internal/domain"
	"catcube/internal/modules/circle/internal/infra"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/pkg/ids"
)

// Repo 应用层仓储能力。
type Repo interface {
	CreateCircle(ctx context.Context, q infra.Querier, c infra.Circle) error
	GetCircle(ctx context.Context, id int64) (infra.Circle, error)
	ListByUser(ctx context.Context, userID int64) ([]infra.Circle, error)
	CountCreatedToday(ctx context.Context, userID int64) (int, error)
	CountOwned(ctx context.Context, userID int64) (int, error)
	SetStatus(ctx context.Context, q infra.Querier, id int64, status int16, dissolved bool) error
	ConvertPermanent(ctx context.Context, q infra.Querier, id int64) error
	AddMember(ctx context.Context, q infra.Querier, id, circleID, userID int64, via int16) (bool, error)
	IsMember(ctx context.Context, circleID, userID int64) (bool, error)
	ListMembers(ctx context.Context, circleID int64) ([]int64, error)
	InsertInvite(ctx context.Context, q infra.Querier, id, circleID, createdBy int64, code string, expire time.Time) error
	GetInvite(ctx context.Context, code string) (int64, time.Time, error)
	CreateDemand(ctx context.Context, q infra.Querier, d infra.Demand, contactEnc []byte) error
	GetDemand(ctx context.Context, id int64) (infra.Demand, error)
	ListDemands(ctx context.Context, circleID int64, limit int) ([]infra.Demand, error)
	CountUserPostsToday(ctx context.Context, userID int64) (int, error)
	CountAnnounceToday(ctx context.Context, circleID int64) (int, error)
	RenewDemand(ctx context.Context, q infra.Querier, id int64, expire time.Time) error
	ExpireDueDemands(ctx context.Context, limit int) ([]int64, error)
	CreateFeed(ctx context.Context, q infra.Querier, id, circleID, authorID int64, text string, images []byte, announce bool) error
	ListDueCircles(ctx context.Context, limit int) ([]int64, error)
}

// Options 构造参数。
type Options struct {
	Repo      Repo
	Pool      *db.Pool
	Bus       *eventbus.Bus
	Policy    *policy.Engine
	Identity  idcontract.Facade
	Encryptor *sensitive.Encryptor
}

// Service circle 应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	bus      *eventbus.Bus
	policy   *policy.Engine
	identity idcontract.Facade
	enc      *sensitive.Encryptor
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, policy: o.Policy, identity: o.Identity, enc: o.Encryptor}
}

const (
	postDailyLimit   = 3
	announceDailyMax = 3
	defaultDemandDays = 30
)

// Create 建圈（已实名；临时圈 7/30/90/180；配额 1/日、同时最多 3 个）。
func (s *Service) Create(ctx context.Context, userID int64, in contract.CreateInput) (contract.CircleBrief, error) {
	if err := domain.ValidateCreate(in.Name, in.Type, in.DurationDays); err != nil {
		return contract.CircleBrief{}, err
	}
	if err := s.requireRealname(ctx, userID); err != nil {
		return contract.CircleBrief{}, err
	}
	n, err := s.repo.CountCreatedToday(ctx, userID)
	if err != nil {
		return contract.CircleBrief{}, err
	}
	if n >= 1 {
		return contract.CircleBrief{}, domain.ErrCircleLimit
	}
	owned, err := s.repo.CountOwned(ctx, userID)
	if err != nil {
		return contract.CircleBrief{}, err
	}
	if owned >= 3 {
		return contract.CircleBrief{}, domain.ErrCircleLimit
	}
	c := infra.Circle{
		ID: int64(ids.New()), Name: in.Name, Intro: in.Intro, CoverFileID: in.CoverFileID,
		RegionCode: in.RegionCode, Industry: in.IndustryCodes, Type: in.Type,
		DurationDays: int16(in.DurationDays), OwnerUserID: userID, OrgID: in.OrgID,
	}
	if in.Type == 1 {
		c.DurationDays = 0
	} else {
		end := time.Now().UTC().AddDate(0, 0, in.DurationDays)
		c.EndAt = &end
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.CreateCircle(ctx, tx, c); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewCircleCreated(ids.New().String(), contract.CirclePayload{
			CircleID: c.ID, Name: c.Name, OwnerID: userID,
		}))
	})
	if err != nil {
		return contract.CircleBrief{}, err
	}
	return toBrief(c), nil
}

// Brief 圈子摘要。
func (s *Service) Brief(ctx context.Context, circleID int64) (contract.CircleBrief, error) {
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return contract.CircleBrief{}, err
	}
	return toBrief(c), nil
}

// RoleOf 我的角色（0 非成员 / 1 圈主 / 2 成员）。
func (s *Service) RoleOf(ctx context.Context, circleID, userID int64) (int16, error) {
	role, _, err := s.roleOf(ctx, circleID, userID)
	return role, err
}

// IsMember 是否圈成员。
func (s *Service) IsMember(ctx context.Context, circleID, userID int64) (bool, error) {
	return s.repo.IsMember(ctx, circleID, userID)
}

// ListMine 我加入/创建的圈子。
func (s *Service) ListMine(ctx context.Context, userID int64) ([]contract.CircleBrief, error) {
	rows, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.CircleBrief, 0, len(rows))
	for _, c := range rows {
		out = append(out, toBrief(c))
	}
	return out, nil
}

// Detail 圈详情（非成员仅见关于本圈由前端裁剪）。
func (s *Service) Detail(ctx context.Context, viewerID, circleID int64) (contract.CircleDetail, error) {
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return contract.CircleDetail{}, err
	}
	role, joined, err := s.roleOf(ctx, circleID, viewerID)
	if err != nil {
		return contract.CircleDetail{}, err
	}
	return contract.CircleDetail{
		CircleBrief: toBrief(c), Intro: c.Intro, CoverFileID: c.CoverFileID, RegionCode: c.RegionCode,
		IndustryCodes: c.Industry, MyRole: role, Joined: joined,
		Recommendable: !time.Now().UTC().Before(c.RecommendableAt),
	}, nil
}

// Apply 申请加入（圈主审）。
func (s *Service) Apply(ctx context.Context, circleID, userID int64) error {
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return err
	}
	if c.Status != contract.CircleActive {
		return domain.ErrEnded
	}
	if err := s.requireRealname(ctx, userID); err != nil {
		return err
	}
	// 申请记录：一期以成员行 status 表达（0 待审/…）；简化：直接入成员（圈主审可在后台处理）
	if _, err := s.repo.AddMember(ctx, s.pool, int64(ids.New()), circleID, userID, contract.ViaApply); err != nil {
		return err
	}
	return s.emitMemberJoined(ctx, circleID, userID, contract.ViaApply)
}

// ApproveApply 圈主通过申请（与 Apply 幂等结合使用）。
func (s *Service) ApproveApply(ctx context.Context, circleID, ownerID, applicantID int64) error {
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return err
	}
	if c.OwnerUserID != ownerID {
		return domain.ErrNoPermission
	}
	if _, err := s.repo.AddMember(ctx, s.pool, int64(ids.New()), circleID, applicantID, contract.ViaApply); err != nil {
		return err
	}
	return s.emitMemberJoined(ctx, circleID, applicantID, contract.ViaApply)
}

// InviteCode 生成邀请码（=圈剩余存续期；可重置旧链作废；圈主与成员均可生成）。
func (s *Service) InviteCode(ctx context.Context, circleID, userID int64) (string, time.Time, error) {
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return "", time.Time{}, err
	}
	if c.Status != contract.CircleActive {
		return "", time.Time{}, domain.ErrEnded
	}
	ok, err := s.isMemberOrOwner(ctx, circleID, userID)
	if err != nil {
		return "", time.Time{}, err
	}
	if !ok {
		return "", time.Time{}, domain.ErrNoPermission
	}
	expire := time.Now().UTC().AddDate(0, 0, 3650) // 常设圈长期有效
	if c.EndAt != nil {
		expire = *c.EndAt
	}
	code := "I" + strconv.FormatInt(int64(ids.New()), 36)
	if err := s.repo.InsertInvite(ctx, s.pool, int64(ids.New()), circleID, userID, code, expire); err != nil {
		return "", time.Time{}, err
	}
	return code, expire, nil
}

// JoinByInvite 凭邀请链接直接加入。
func (s *Service) JoinByInvite(ctx context.Context, userID int64, code string) (contract.CircleBrief, error) {
	circleID, expire, err := s.repo.GetInvite(ctx, code)
	if err != nil || time.Now().UTC().After(expire) {
		return contract.CircleBrief{}, domain.ErrInviteInvalid
	}
	if err := s.requireRealname(ctx, userID); err != nil {
		return contract.CircleBrief{}, err
	}
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return contract.CircleBrief{}, err
	}
	if c.Status != contract.CircleActive {
		return contract.CircleBrief{}, domain.ErrEnded
	}
	if _, err := s.repo.AddMember(ctx, s.pool, int64(ids.New()), circleID, userID, contract.ViaInvite); err != nil {
		return contract.CircleBrief{}, err
	}
	_ = s.emitMemberJoined(ctx, circleID, userID, contract.ViaInvite)
	return toBrief(c), nil
}

// Members 成员 ID 列表。
func (s *Service) Members(ctx context.Context, circleID int64) ([]int64, error) {
	return s.repo.ListMembers(ctx, circleID)
}

// Dissolve 圈主解散（永久删除，通知全员由事件消费方处理）。
func (s *Service) Dissolve(ctx context.Context, circleID, userID int64) error {
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return err
	}
	if c.OwnerUserID != userID {
		return domain.ErrNoPermission
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.SetStatus(ctx, tx, circleID, contract.CircleDissolved, true); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewCircleDissolved(ids.New().String(), contract.CirclePayload{
			CircleID: circleID, Name: c.Name, UserID: userID,
		}))
	})
	return err
}

// ConvertToPermanent 已结束转常设（圈主）。
func (s *Service) ConvertToPermanent(ctx context.Context, circleID, userID int64) error {
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return err
	}
	if c.OwnerUserID != userID {
		return domain.ErrNoPermission
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.ConvertPermanent(ctx, tx, circleID)
	})
	return err
}

// EndDue 到期转「已结束」（只读不删；SchedulerJob）。
func (s *Service) EndDue(ctx context.Context) error {
	idsDue, err := s.repo.ListDueCircles(ctx, 200)
	if err != nil {
		return err
	}
	for _, id := range idsDue {
		c, err := s.repo.GetCircle(ctx, id)
		if err != nil {
			continue
		}
		_ = s.pool.InTx(ctx, func(tx db.Tx) error {
			if err := s.repo.SetStatus(ctx, tx, id, contract.CircleEnded, false); err != nil {
				return err
			}
			return s.publish(ctx, tx, contract.NewCircleEnded(ids.New().String(), contract.CirclePayload{
				CircleID: id, Name: c.Name, OwnerID: c.OwnerUserID,
			}))
		})
	}
	return nil
}

// ---- 圈内发布（§6）----

// PublishDemand 发布需求（每人每日跨圈合计 3 条；VIP 可选全平台）。
func (s *Service) PublishDemand(ctx context.Context, circleID, userID int64, in contract.DemandInput) (contract.Demand, error) {
	if err := domain.ValidateDemand(in.Title, in.Content); err != nil {
		return contract.Demand{}, err
	}
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return contract.Demand{}, err
	}
	if c.Status != contract.CircleActive {
		return contract.Demand{}, domain.ErrEnded
	}
	if ok, err := s.isMemberOrOwner(ctx, circleID, userID); err != nil || !ok {
		return contract.Demand{}, domain.ErrNoPermission
	}
	n, err := s.repo.CountUserPostsToday(ctx, userID)
	if err != nil {
		return contract.Demand{}, err
	}
	if n >= postDailyLimit {
		return contract.Demand{}, domain.ErrPostQuota
	}
	scope := in.Scope
	if scope != 2 {
		scope = 1
	} else if !s.hasCap(ctx, userID, "member.active") {
		scope = 1 // 非会员仅本圈
	}
	days := in.Days
	if days <= 0 {
		days = defaultDemandDays
	}
	var enc []byte
	if in.ContactPhone != "" {
		if s.enc != nil {
			if v, err := s.enc.Encrypt([]byte(in.ContactPhone)); err == nil {
				enc = v
			}
		} else {
			enc = []byte(in.ContactPhone)
		}
	}
	d := infra.Demand{
		ID: int64(ids.New()), CircleID: circleID, AuthorID: userID, Title: in.Title, Content: in.Content,
		DemandType: in.DemandType, RegionCode: in.RegionCode, Industry: in.IndustryCodes,
		ExpireAt: time.Now().UTC().AddDate(0, 0, days), Scope: scope,
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.CreateDemand(ctx, tx, d, enc); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewDemandPublished(ids.New().String(), contract.DemandPayload{
			CircleID: circleID, DemandID: d.ID, AuthorID: userID, Title: d.Title, Scope: scope,
		}))
	})
	if err != nil {
		return contract.Demand{}, err
	}
	out := toDemand(d)
	if s.hasCap(ctx, userID, "member.active") {
		out.ContactPhone = in.ContactPhone
	}
	return out, nil
}

// ListDemands 需求列表（联系方式会员可见）。
func (s *Service) ListDemands(ctx context.Context, circleID, viewerID int64, limit int) ([]contract.Demand, error) {
	rows, err := s.repo.ListDemands(ctx, circleID, limit)
	if err != nil {
		return nil, err
	}
	member := s.hasCap(ctx, viewerID, "member.active")
	out := make([]contract.Demand, 0, len(rows))
	for _, d := range rows {
		cd := toDemand(d)
		if member && s.enc != nil {
			if enc, err := s.repoDemandContact(ctx, d.ID); err == nil && len(enc) > 0 {
				if plain, err := s.enc.Decrypt(enc); err == nil {
					cd.ContactPhone = string(plain)
				}
			}
		}
		out = append(out, cd)
	}
	return out, nil
}

// RenewDemand 一键续期 30 天（作者）。
func (s *Service) RenewDemand(ctx context.Context, demandID, userID int64) error {
	d, err := s.repo.GetDemand(ctx, demandID)
	if err != nil {
		return err
	}
	if d.AuthorID != userID {
		return domain.ErrNoPermission
	}
	return s.repo.RenewDemand(ctx, s.pool, demandID, time.Now().UTC().AddDate(0, 0, defaultDemandDays))
}

// PublishFeed 圈内动态/公告（公告仅圈主、每圈每日 ≤3 条不占个人额度）。
func (s *Service) PublishFeed(ctx context.Context, circleID, userID int64, text string, imageFileIDs []int64, announcement bool) (int64, error) {
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return 0, err
	}
	if c.Status != contract.CircleActive {
		return 0, domain.ErrEnded
	}
	if announcement {
		if c.OwnerUserID != userID {
			return 0, domain.ErrNoPermission
		}
		n, err := s.repo.CountAnnounceToday(ctx, circleID)
		if err != nil {
			return 0, err
		}
		if n >= announceDailyMax {
			return 0, domain.ErrAnnounceQuota
		}
	} else {
		if ok, err := s.isMemberOrOwner(ctx, circleID, userID); err != nil || !ok {
			return 0, domain.ErrNoPermission
		}
		n, err := s.repo.CountUserPostsToday(ctx, userID)
		if err != nil {
			return 0, err
		}
		if n >= postDailyLimit {
			return 0, domain.ErrPostQuota
		}
	}
	imgs, _ := json.Marshal(imageFileIDs)
	id := int64(ids.New())
	if err := s.repo.CreateFeed(ctx, s.pool, id, circleID, userID, text, imgs, announcement); err != nil {
		return 0, err
	}
	return id, nil
}

// ExpireDemands 需求到期沉底标「已过期」（SchedulerJob）。
func (s *Service) ExpireDemands(ctx context.Context) error {
	idsDue, err := s.repo.ExpireDueDemands(ctx, 200)
	if err != nil {
		return err
	}
	for _, id := range idsDue {
		d, err := s.repo.GetDemand(ctx, id)
		if err != nil {
			continue
		}
		_ = s.pool.InTx(ctx, func(tx db.Tx) error {
			return s.publish(ctx, tx, contract.NewDemandExpired(ids.New().String(), contract.DemandPayload{
				CircleID: d.CircleID, DemandID: d.ID, AuthorID: d.AuthorID, Title: d.Title,
			}))
		})
	}
	return nil
}

// ---- 内部 ----

func (s *Service) roleOf(ctx context.Context, circleID, userID int64) (role int16, joined bool, err error) {
	c, err := s.repo.GetCircle(ctx, circleID)
	if err != nil {
		return 0, false, err
	}
	if c.OwnerUserID == userID {
		return 1, true, nil
	}
	m, err := s.repo.IsMember(ctx, circleID, userID)
	if err != nil {
		return 0, false, err
	}
	if m {
		return 2, true, nil
	}
	return 0, false, nil
}

func (s *Service) isMemberOrOwner(ctx context.Context, circleID, userID int64) (bool, error) {
	role, joined, err := s.roleOf(ctx, circleID, userID)
	if err != nil {
		return false, err
	}
	return joined && role > 0, nil
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

func (s *Service) hasCap(ctx context.Context, userID int64, cap string) bool {
	if s.policy == nil {
		return false
	}
	ok, err := s.policy.Has(ctx, s.policy.NewActor(userID, 1), cap)
	return err == nil && ok
}

func (s *Service) emitMemberJoined(ctx context.Context, circleID, userID int64, via int16) error {
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.publish(ctx, tx, contract.NewMemberJoined(ids.New().String(), contract.MemberPayload{
			CircleID: circleID, UserID: userID, Via: via,
		}))
	})
}

func (s *Service) publish(ctx context.Context, tx db.Tx, e eventbus.Event) error {
	if s.bus == nil {
		return nil
	}
	return s.bus.Publish(ctx, tx, e)
}

// repoDemandContact 读取需求联系方式密文（会员可见）。
func (s *Service) repoDemandContact(ctx context.Context, demandID int64) ([]byte, error) {
	var enc []byte
	err := s.pool.QueryRow(ctx, `SELECT contact_phone_enc FROM cir_demand WHERE id=$1`, demandID).Scan(&enc)
	return enc, err
}

func toBrief(c infra.Circle) contract.CircleBrief {
	return contract.CircleBrief{ID: c.ID, Name: c.Name, Type: c.Type, Status: c.Status,
		OwnerUserID: c.OwnerUserID, MemberCount: c.MemberCount, EndAt: c.EndAt}
}

func toDemand(d infra.Demand) contract.Demand {
	return contract.Demand{ID: d.ID, CircleID: d.CircleID, AuthorID: d.AuthorID, Title: d.Title,
		Content: d.Content, DemandType: d.DemandType, RegionCode: d.RegionCode, IndustryCodes: d.Industry,
		ExpireAt: d.ExpireAt, Scope: d.Scope, Status: d.Status, CreatedAt: d.CreatedAt}
}

