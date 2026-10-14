// Package app membership 应用服务：报价、开通（下单）、履约（开通/续期）、到期回落、子账号。
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/settings"
	"catcube/internal/kernel/statemachine"
	idcontract "catcube/internal/modules/identity/contract"
	memcontract "catcube/internal/modules/membership/contract"
	"catcube/internal/modules/membership/internal/domain"
	"catcube/internal/modules/membership/internal/infra"
	tradecontract "catcube/internal/modules/trade/contract"
	verifycontract "catcube/internal/modules/verify/contract"
	"catcube/pkg/ids"
)

// 配置键（settings；后台可配、带版本）。
const (
	KeyYearFeeVIP        = "membership.year_fee_vip"
	KeyYearFeeEnterprise = "membership.year_fee_enterprise"
	KeyDiscount3Y        = "membership.discount_3y"
	KeyDiscount5Y        = "membership.discount_5y"
)

// Schemas 配置声明（后台据此生成配置界面）。
var Schemas = []settings.Schema{
	{Key: KeyYearFeeVIP, Type: settings.TypeFen, Default: "9900", Description: "VIP 年费（分）"},
	{Key: KeyYearFeeEnterprise, Type: settings.TypeFen, Default: "29900", Description: "企业版年费（分）"},
	{Key: KeyDiscount3Y, Type: settings.TypeRate, Default: "9000", Description: "3 年折扣系数（万分比）"},
	{Key: KeyDiscount5Y, Type: settings.TypeRate, Default: "8000", Description: "5 年折扣系数（万分比）"},
}

// Repo 应用层仓储能力。
type Repo interface {
	GetMember(ctx context.Context, userID int64) (infra.Member, error)
	UpsertMember(ctx context.Context, tx db.Tx, m infra.Member) error
	ListExpired(ctx context.Context, limit int) ([]infra.Member, error)
	MarkExpired(ctx context.Context, userID int64) error
	ListSubs(ctx context.Context, masterID int64) ([]infra.SubAccount, error)
	CountActiveSubs(ctx context.Context, masterID int64) (int, error)
	InsertSub(ctx context.Context, tx db.Tx, id, masterID, subUserID int64, expiresAt time.Time) error
	RemoveSub(ctx context.Context, tx db.Tx, masterID, subUserID int64) error
}

// Options 构造参数。
type Options struct {
	Repo     Repo
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Trade    tradecontract.Facade
	Verify   verifycontract.Facade
	Identity idcontract.Facade
	Settings *settings.Registry
	Policy   *policy.Engine
}

// Service membership 应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	bus      *eventbus.Bus
	trade    tradecontract.Facade
	verify   verifycontract.Facade
	identity idcontract.Facade
	settings *settings.Registry
	policy   *policy.Engine
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, trade: o.Trade, verify: o.Verify,
		identity: o.Identity, settings: o.Settings, policy: o.Policy}
}

// Quote 报价（多年价=年费×年限×系数）。
func (s *Service) Quote(ctx context.Context, level int16, years int) (memcontract.Quote, error) {
	if !domain.ValidLevel(level) {
		return memcontract.Quote{}, domain.ErrLevelUnknown
	}
	if !domain.ValidYears(years) {
		return memcontract.Quote{}, domain.ErrYearsInvalid
	}
	unit := s.unitFen(ctx, level)
	coeff := s.discount(ctx, years)
	original, total := domain.Quote(unit, years, coeff)
	return memcontract.Quote{Level: level, Years: years, UnitFen: unit, OriginalFen: original, DiscountRate: coeff, TotalFen: total}, nil
}

// Open 开通/续费：企业版资格校验 → 创建 trade 订单。
func (s *Service) Open(ctx context.Context, userID int64, level int16, years int) (memcontract.OrderResult, error) {
	q, err := s.Quote(ctx, level, years)
	if err != nil {
		return memcontract.OrderResult{}, err
	}
	if level == memcontract.LevelEnterprise {
		brief, err := s.identity.UserBrief(ctx, userID)
		if err != nil || brief.IdentityType != 2 {
			return memcontract.OrderResult{}, domain.ErrEnterpriseCert
		}
		if ok, _ := s.hasCap(ctx, userID, "verified"); !ok {
			return memcontract.OrderResult{}, domain.ErrEnterpriseCert
		}
	}
	snapshot := map[string]any{
		"level": level, "years": years, "unit_fen": q.UnitFen,
		"original_fen": q.OriginalFen, "discount_rate": q.DiscountRate, "total_fen": q.TotalFen,
	}
	order, err := s.trade.CreateOrder(ctx, userID, tradecontract.CreateOrderInput{
		BizType: "member", BizRef: fmt.Sprintf("level:%d", level), BuyerID: userID,
		AmountFen: q.TotalFen, PriceSnapshot: snapshot, Subject: "猫平方会员",
	}, "member:"+fmt.Sprint(ids.New()))
	if err != nil {
		return memcontract.OrderResult{}, err
	}
	return memcontract.OrderResult{OrderID: order.ID, AmountFen: q.TotalFen, OriginalFen: q.OriginalFen}, nil
}

// Fulfill 履约（trade.OrderFulfiller）：OrderPaid 后开通/续期，幂等。
func (s *Service) Fulfill(ctx context.Context, order tradecontract.Order) error {
	if order.BizType != "member" {
		return nil
	}
	var snap struct {
		Level int16 `json:"level"`
		Years int   `json:"years"`
	}
	_ = json.Unmarshal(order.PriceSnapshot, &snap)
	if snap.Years <= 0 {
		snap.Years = 1
	}
	// 幂等：同一订单已履约则跳过
	cur, err := s.repo.GetMember(ctx, order.BuyerID)
	has := err == nil
	if err != nil && !errors.Is(err, infra.ErrNotFound) {
		return err
	}
	if has && cur.LastOrderID == order.ID {
		return nil
	}
	base := time.Now().UTC()
	if has && cur.ExpiresAt.After(base) && cur.Status == 1 {
		base = cur.ExpiresAt // 续期：在现有到期时间上叠加
	}
	if _, err := statemachine.NewEngine(nil).Apply(ctx, domain.Machine, order.BuyerID, domain.StateOf(cur.Status, has), domain.EventActivate, "system"); err != nil {
		return err
	}
	newExpire := base.Add(time.Duration(snap.Years) * 365 * 24 * time.Hour)
	m := infra.Member{UserID: order.BuyerID, Level: snap.Level, Status: 1, ExpiresAt: newExpire,
		LastOrderID: order.ID, PriceSnapshot: order.PriceSnapshot}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.UpsertMember(ctx, tx, m)
	}); err != nil {
		return err
	}
	return s.emit(ctx, memcontract.NewMemberActivated(ids.New().String(), memcontract.MemberPayload{
		UserID: order.BuyerID, Level: snap.Level, ExpiresAt: newExpire.Format(time.RFC3339),
	}))
}

// Me 我的会员状态。
func (s *Service) Me(ctx context.Context, userID int64) (memcontract.Member, bool, error) {
	m, err := s.repo.GetMember(ctx, userID)
	if errors.Is(err, infra.ErrNotFound) {
		return memcontract.Member{UserID: userID}, false, nil
	}
	if err != nil {
		return memcontract.Member{}, false, err
	}
	return memcontract.Member{UserID: m.UserID, Level: m.Level, Status: m.Status, ExpiresAt: m.ExpiresAt}, true, nil
}

// LevelOf 生效会员的等级。
func (s *Service) LevelOf(ctx context.Context, userID int64) (int16, bool, error) {
	m, err := s.repo.GetMember(ctx, userID)
	if errors.Is(err, infra.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	active := m.Status == 1 && m.ExpiresAt.After(time.Now().UTC())
	return m.Level, active, nil
}

// SubAccounts 子账号列表。
func (s *Service) SubAccounts(ctx context.Context, masterID int64) ([]memcontract.SubAccount, error) {
	rows, err := s.repo.ListSubs(ctx, masterID)
	if err != nil {
		return nil, err
	}
	out := make([]memcontract.SubAccount, 0, len(rows))
	for _, a := range rows {
		out = append(out, memcontract.SubAccount{ID: a.ID, MasterID: a.MasterID, SubUserID: a.SubUserID, Status: a.Status, InvitedAt: a.InvitedAt})
	}
	return out, nil
}

// InviteSub 邀请子账号（企业版主账号；须目标已实名；上限 3）。
func (s *Service) InviteSub(ctx context.Context, masterID, subUserID int64) error {
	m, err := s.repo.GetMember(ctx, masterID)
	if errors.Is(err, infra.ErrNotFound) || (err == nil && (m.Level != memcontract.LevelEnterprise || m.Status != 1)) {
		return domain.ErrNotEnterprise
	}
	if err != nil {
		return err
	}
	ok, err := s.hasCap(ctx, subUserID, "realname")
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrSubNotRealname
	}
	n, err := s.repo.CountActiveSubs(ctx, masterID)
	if err != nil {
		return err
	}
	if n >= 3 {
		return domain.ErrSubLimit
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.InsertSub(ctx, tx, int64(ids.New()), masterID, subUserID, m.ExpiresAt)
	}); err != nil {
		return err
	}
	return s.emit(ctx, memcontract.NewSubAccountChanged(ids.New().String(), memcontract.SubAccountPayload{MasterID: masterID, SubUserID: subUserID, Action: "invite"}))
}

// RemoveSub 移除子账号。
func (s *Service) RemoveSub(ctx context.Context, masterID, subUserID int64) error {
	m, err := s.repo.GetMember(ctx, masterID)
	if err != nil || m.Level != memcontract.LevelEnterprise {
		return domain.ErrNotEnterprise
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.RemoveSub(ctx, tx, masterID, subUserID)
	}); err != nil {
		return err
	}
	return s.emit(ctx, memcontract.NewSubAccountChanged(ids.New().String(), memcontract.SubAccountPayload{MasterID: masterID, SubUserID: subUserID, Action: "remove"}))
}

// RefundQuote 退款金额（剩余完整年数×年费×系数，取订单快照）。
func (s *Service) RefundQuote(ctx context.Context, orderID int64) (int64, error) {
	order, err := s.trade.GetOrder(ctx, orderID)
	if err != nil {
		return 0, err
	}
	var snap struct {
		UnitFen      int64 `json:"unit_fen"`
		Years        int   `json:"years"`
		DiscountRate int   `json:"discount_rate"`
	}
	_ = json.Unmarshal(order.PriceSnapshot, &snap)
	m, err := s.repo.GetMember(ctx, order.BuyerID)
	if err != nil {
		return 0, domain.ErrNotActive
	}
	return domain.RefundAmount(snap.UnitFen, snap.Years, snap.DiscountRate, m.ExpiresAt, time.Now().UTC()), nil
}

// ExpireDue 到期回落（SchedulerJob）。
func (s *Service) ExpireDue(ctx context.Context) error {
	list, err := s.repo.ListExpired(ctx, 200)
	if err != nil {
		return err
	}
	for _, m := range list {
		if _, err := statemachine.NewEngine(nil).Apply(ctx, domain.Machine, m.UserID, domain.StateActive, domain.EventExpire, "system"); err != nil {
			continue
		}
		if err := s.repo.MarkExpired(ctx, m.UserID); err != nil {
			continue
		}
		_ = s.emit(ctx, memcontract.NewMemberExpired(ids.New().String(), memcontract.MemberPayload{UserID: m.UserID, Level: m.Level}))
	}
	return nil
}

// ---- 内部 ----

func (s *Service) hasCap(ctx context.Context, userID int64, capability string) (bool, error) {
	if s.policy == nil {
		return false, nil
	}
	return s.policy.Has(ctx, s.policy.NewActor(userID, 1), capability)
}

func (s *Service) unitFen(ctx context.Context, level int16) int64 {
	key := KeyYearFeeVIP
	def := int64(9900)
	if level == memcontract.LevelEnterprise {
		key = KeyYearFeeEnterprise
		def = 29900
	}
	if s.settings == nil {
		return def
	}
	v, err := s.settings.Int(ctx, 1, key)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func (s *Service) discount(ctx context.Context, years int) int {
	if s.settings == nil {
		return domain.DiscountCoeff(years, 9000, 8000)
	}
	c3, _ := s.settings.Int(ctx, 1, KeyDiscount3Y)
	c5, _ := s.settings.Int(ctx, 1, KeyDiscount5Y)
	if c3 <= 0 {
		c3 = 9000
	}
	if c5 <= 0 {
		c5 = 8000
	}
	return domain.DiscountCoeff(years, int(c3), int(c5))
}

func (s *Service) emit(ctx context.Context, e eventbus.Event) error {
	if s.bus == nil {
		return nil
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.bus.Publish(ctx, tx, e)
	})
}
