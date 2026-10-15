// Package app profile 应用服务：名片读写、画像完成度、对外展示过滤、扩展点实现。
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/sensitive"
	idcontract "catcube/internal/modules/identity/contract"
	prfcontract "catcube/internal/modules/profile/contract"
	"catcube/internal/modules/profile/internal/domain"
	"catcube/internal/modules/profile/internal/infra"
	dictcontract "catcube/internal/shared/dict/contract"
	mediacontract "catcube/internal/shared/media/contract"
	"catcube/pkg/ids"
)

// Repo 应用层所需仓储能力（PG 实现见 infra.Store）。
type Repo interface {
	GetByUserID(ctx context.Context, userID int64) (infra.Card, error)
	CreateDefault(ctx context.Context, tx db.Tx, userID int64, identityType int16, name string) error
	Save(ctx context.Context, tx db.Tx, c infra.Card) error
}

// UpdateInput 名片更新入参（PUT 全量覆盖；联系方式为明文，服务端加密）。
type UpdateInput struct {
	SubjectTypeCode      string          `json:"subject_type_code"`
	Name                 string          `json:"name"`
	AvatarFileID         int64           `json:"avatar_file_id"`
	RegionCode           string          `json:"region_code"`
	IndustryCodes        []string        `json:"industry_codes"`
	NeedCodes            []string        `json:"need_codes"`
	OfferCodes           []string        `json:"offer_codes"`
	AdvantageField       string          `json:"advantage_field"`
	Background           string          `json:"background"`
	TargetCustomers      string          `json:"target_customers"`
	TargetChannels       string          `json:"target_channels"`
	QualificationFileIDs []int64         `json:"qualification_file_ids"`
	CoreStrength         string          `json:"core_strength"`
	Intro                string          `json:"intro"`
	ContactPhone         string          `json:"contact_phone"`
	Visibility           map[string]bool `json:"visibility_settings"`
}

// Options 构造参数。
type Options struct {
	Repo     Repo
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Media    mediacontract.Facade
	Dict     dictcontract.Facade
	Identity idcontract.Facade
	Encrypt  *sensitive.Encryptor
}

// Service 名片应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	bus      *eventbus.Bus
	media    mediacontract.Facade
	dict     dictcontract.Facade
	identity idcontract.Facade
	enc      *sensitive.Encryptor
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, media: o.Media,
		dict: o.Dict, identity: o.Identity, enc: o.Encrypt}
}

// EnsureCard 确保名片存在（注册/首次访问时懒建，返回账号摘要）。
func (s *Service) EnsureCard(ctx context.Context, userID int64) (idcontract.UserBrief, error) {
	brief, err := s.identity.UserBrief(ctx, userID)
	if err != nil {
		return idcontract.UserBrief{}, err
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.CreateDefault(ctx, tx, userID, brief.IdentityType, brief.Nickname)
	})
	return brief, err
}

// GetMyCard 本人名片（不做展示过滤）。
func (s *Service) GetMyCard(ctx context.Context, userID int64) (prfcontract.Card, error) {
	c, err := s.repo.GetByUserID(ctx, userID)
	if errors.Is(err, infra.ErrNotFound) {
		if _, e := s.EnsureCard(ctx, userID); e != nil {
			return prfcontract.Card{}, e
		}
		c, err = s.repo.GetByUserID(ctx, userID)
	}
	if err != nil {
		return prfcontract.Card{}, err
	}
	return toContract(c), nil
}

// UpdateCard 更新名片（校验 → 锁定检查 → 标签校验 → 加密 → 保存 → 同事务发事件）。
func (s *Service) UpdateCard(ctx context.Context, userID int64, in UpdateInput) (prfcontract.Card, error) {
	cur, err := s.repo.GetByUserID(ctx, userID)
	if errors.Is(err, infra.ErrNotFound) {
		if _, e := s.EnsureCard(ctx, userID); e != nil {
			return prfcontract.Card{}, e
		}
		cur, err = s.repo.GetByUserID(ctx, userID)
	}
	if err != nil {
		return prfcontract.Card{}, err
	}

	next := cur
	next.SubjectTypeCode = in.SubjectTypeCode
	next.Name = strings.TrimSpace(in.Name)
	next.AvatarFileID = in.AvatarFileID
	next.RegionCode = in.RegionCode
	next.IndustryCodes = in.IndustryCodes
	next.NeedCodes = in.NeedCodes
	next.OfferCodes = in.OfferCodes
	next.AdvantageField = in.AdvantageField
	next.Background = in.Background
	next.TargetCustomers = in.TargetCustomers
	next.TargetChannels = in.TargetChannels
	next.CoreStrength = in.CoreStrength
	next.Intro = in.Intro
	if in.Visibility != nil {
		next.Visibility = domain.Visibility(in.Visibility)
	}

	dc := toDomain(next)
	if err := domain.Validate(dc); err != nil {
		return prfcontract.Card{}, err
	}
	// 认证主体信息锁定（locked_fields 由 verify 认证通过后写入）
	if locked := domain.LockedChange(toDomain(cur), dc, cur.LockedFields); locked != "" {
		return prfcontract.Card{}, fmt.Errorf("%w: %s", domain.ErrFieldLocked, locked)
	}
	// 标签合法性（行业/区域/需求/供给统一校验）
	if s.dict != nil {
		if err := s.dict.ValidateTagSet(ctx, dictcontract.TagSet{
			Industry: in.IndustryCodes, Region: in.RegionCode, Need: in.NeedCodes, Offer: in.OfferCodes,
		}); err != nil {
			return prfcontract.Card{}, fmt.Errorf("%w: %v", domain.ErrInvalidTag, err)
		}
	}
	// 联系方式：非空才更新（明文加密存储，永不回传）
	if strings.TrimSpace(in.ContactPhone) != "" {
		if s.enc == nil {
			next.ContactPhoneEnc = []byte(in.ContactPhone)
		} else {
			enc, e := s.enc.Encrypt([]byte(in.ContactPhone))
			if e != nil {
				return prfcontract.Card{}, e
			}
			next.ContactPhoneEnc = enc
		}
	}
	// 完成度与层级
	_, _, tier := domain.Completion(dc)
	next.ProfileTier = tier
	next.SearchDocVersion = cur.SearchDocVersion + 1
	next.QualificationIDs = in.QualificationFileIDs

	// 资质图片引用计数（新增绑定 / 移除解绑），接 media.Bind
	if s.media != nil {
		added, removed := diffIDs(cur.QualificationIDs, in.QualificationFileIDs)
		owner := fmt.Sprintf("user:%d", userID)
		for _, id := range added {
			_ = s.media.Bind(ctx, id, owner)
		}
		for _, id := range removed {
			_ = s.media.Unbind(ctx, id, owner)
		}
	}

	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.Save(ctx, tx, next); err != nil {
			return err
		}
		if s.bus == nil {
			return nil
		}
		return s.bus.Publish(ctx, tx, buildEvent(next))
	}); err != nil {
		return prfcontract.Card{}, err
	}
	return toContract(next), nil
}

// buildEvent 构造 ProfileUpdated / OrgCardUpdated。
func buildEvent(c infra.Card) eventbus.Event {
	payload := prfcontract.ProfileUpdatedPayload{
		UserID:           c.UserID,
		SubjectTypeCode:  c.SubjectTypeCode,
		Name:             c.Name,
		ProfileTier:      c.ProfileTier,
		SearchDocVersion: c.SearchDocVersion,
		Match: prfcontract.MatchProfile{
			Region: c.RegionCode, Industry: c.IndustryCodes, Need: c.NeedCodes, Offer: c.OfferCodes,
		},
	}
	if c.IdentityType == 2 {
		return prfcontract.NewOrgCardUpdated(ids.New().String(), payload)
	}
	return prfcontract.NewProfileUpdated(ids.New().String(), payload)
}

// Completion 画像完成度。
func (s *Service) Completion(ctx context.Context, userID int64) (prfcontract.Completion, error) {
	c, err := s.repo.GetByUserID(ctx, userID)
	if errors.Is(err, infra.ErrNotFound) {
		return prfcontract.Completion{Complete: false, Missing: []string{"region", "industry", "need", "offer", "intro"}, Tier: 1}, nil
	}
	if err != nil {
		return prfcontract.Completion{}, err
	}
	missing, complete, tier := domain.Completion(toDomain(c))
	return prfcontract.Completion{Complete: complete, Missing: missing, Tier: tier}, nil
}

// PublicCard 他人名片：按对外展示设置过滤（敏感字段默认不输出）。
func (s *Service) PublicCard(ctx context.Context, userID int64) (prfcontract.Card, error) {
	c, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return prfcontract.Card{}, err
	}
	d := domain.Public(toDomain(c), c.Visibility)
	c.Name = d.Name
	c.RegionCode = d.RegionCode
	c.IndustryCodes = d.IndustryCodes
	c.NeedCodes = d.NeedCodes
	c.OfferCodes = d.OfferCodes
	c.AdvantageField = d.AdvantageField
	c.Background = d.Background
	c.TargetCustomers = d.TargetCustomers
	c.TargetChannels = d.TargetChannels
	c.CoreStrength = d.CoreStrength
	c.Intro = d.Intro
	c.ContactPhoneEnc = nil
	return toContract(c), nil
}

// OrgBrief 摘要。
func (s *Service) OrgBrief(ctx context.Context, userID int64) (prfcontract.OrgBrief, error) {
	c, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return prfcontract.OrgBrief{}, err
	}
	return prfcontract.OrgBrief{UserID: c.UserID, Name: c.Name, AvatarID: c.AvatarFileID, Region: c.RegionCode}, nil
}

// ContactPhone 取联系方式明文（未配置返回空串）。会员门槛由 bff 判定后调用。
func (s *Service) ContactPhone(ctx context.Context, userID int64) (string, error) {
	c, err := s.repo.GetByUserID(ctx, userID)
	if errors.Is(err, infra.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(c.ContactPhoneEnc) == 0 {
		return "", nil
	}
	if s.enc == nil {
		return string(c.ContactPhoneEnc), nil // 未配置密钥（开发）：明文占位
	}
	plain, err := s.enc.Decrypt(c.ContactPhoneEnc)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// Extract 实现 trust.ReviewSubjectProvider：取名片审核文本与图片。
func (s *Service) Extract(ctx context.Context, userID int64) (string, []string, error) {
	c, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	text := strings.Join([]string{c.Name, c.Intro, c.CoreStrength}, "\n")
	var images []string
	if c.AvatarFileID != 0 {
		images = append(images, fmt.Sprintf("%d", c.AvatarFileID))
	}
	for _, id := range c.QualificationIDs {
		images = append(images, fmt.Sprintf("%d", id))
	}
	return text, images, nil
}

// ApplyVerdict 实现 trust.ReviewSubjectProvider：名片当前不因审核结论落库（无审核态字段），
// 保留开口供后续「审核中/驳回」名片态扩展。
func (s *Service) ApplyVerdict(_ context.Context, _ int64, _ bool, _ string) error { return nil }

// SubjectType 实现 trust.ReviewSubjectProvider。
func (s *Service) SubjectType() string { return "profile" }

// HandleUserRegistered identity.UserRegistered 订阅：建默认名片（第一层）。
func (s *Service) HandleUserRegistered(ctx context.Context, e eventbus.Event) error {
	var p struct {
		UserID int64 `json:"user_id,string"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	if p.UserID == 0 {
		return nil
	}
	_, err := s.EnsureCard(ctx, p.UserID)
	return err
}

// ---- 映射辅助 ----

func toDomain(c infra.Card) domain.Card {
	return domain.Card{
		IdentityType: c.IdentityType, SubjectTypeCode: c.SubjectTypeCode, Name: c.Name,
		RegionCode: c.RegionCode, IndustryCodes: c.IndustryCodes, NeedCodes: c.NeedCodes,
		OfferCodes: c.OfferCodes, AdvantageField: c.AdvantageField, Background: c.Background,
		TargetCustomers: c.TargetCustomers, TargetChannels: c.TargetChannels,
		CoreStrength: c.CoreStrength, Intro: c.Intro, HasContact: len(c.ContactPhoneEnc) > 0,
	}
}

func toContract(c infra.Card) prfcontract.Card {
	return prfcontract.Card{
		UserID: c.UserID, SubjectTypeCode: c.SubjectTypeCode, IdentityType: c.IdentityType,
		Name: c.Name, AvatarFileID: c.AvatarFileID, RegionCode: c.RegionCode,
		IndustryCodes: c.IndustryCodes, NeedCodes: c.NeedCodes, OfferCodes: c.OfferCodes,
		AdvantageField: c.AdvantageField, Background: c.Background,
		TargetCustomers: c.TargetCustomers, TargetChannels: c.TargetChannels,
		CoreStrength: c.CoreStrength, Intro: c.Intro, ProfileTier: c.ProfileTier,
		HasContact: len(c.ContactPhoneEnc) > 0,
	}
}

func diffIDs(oldIDs, newIDs []int64) (added, removed []int64) {
	oldSet := map[int64]bool{}
	for _, id := range oldIDs {
		oldSet[id] = true
	}
	newSet := map[int64]bool{}
	for _, id := range newIDs {
		newSet[id] = true
		if !oldSet[id] {
			added = append(added, id)
		}
	}
	for _, id := range oldIDs {
		if !newSet[id] {
			removed = append(removed, id)
		}
	}
	return added, removed
}
