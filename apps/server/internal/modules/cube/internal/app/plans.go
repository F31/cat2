// plans.go cube 订阅三档与分摊：settings 后台可配；开通/续期经 trade 履约（幂等）。
package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/settings"
	"catcube/internal/modules/cube/contract"
	"catcube/internal/modules/cube/internal/domain"
	"catcube/internal/modules/cube/internal/infra"
	tradecontract "catcube/internal/modules/trade/contract"
	"catcube/pkg/ids"
)

// 配置键（全部后台可配）。
const (
	KeyPlan1Price  = "cube.plan.1.price_fen"
	KeyPlan2Price  = "cube.plan.2.price_fen"
	KeyPlan3Price  = "cube.plan.3.price_fen"
	KeyPlan1Cube   = "cube.plan.1.cube_limit"
	KeyPlan2Cube   = "cube.plan.2.cube_limit"
	KeyPlan3Cube   = "cube.plan.3.cube_limit"
	KeyPlan1Member = "cube.plan.1.member_limit"
	KeyPlan2Member = "cube.plan.2.member_limit"
	KeyPlan3Member = "cube.plan.3.member_limit"
)

// Schemas 订阅三档配置声明（后台自动生成配置界面）。
var Schemas = []settings.Schema{
	{Key: KeyPlan1Price, Type: settings.TypeFen, Default: "0", Description: "基础档价格（分，免费）"},
	{Key: KeyPlan2Price, Type: settings.TypeFen, Default: "99900", Description: "进阶档年费（分）"},
	{Key: KeyPlan3Price, Type: settings.TypeFen, Default: "299900", Description: "高阶档年费（分）"},
	{Key: KeyPlan1Cube, Type: settings.TypeInt, Default: "1", Description: "基础档可加入立方数"},
	{Key: KeyPlan2Cube, Type: settings.TypeInt, Default: "3", Description: "进阶档可加入立方数"},
	{Key: KeyPlan3Cube, Type: settings.TypeInt, Default: "0", Description: "高阶档可加入立方数（0=不限）"},
	{Key: KeyPlan1Member, Type: settings.TypeInt, Default: "10", Description: "基础档单立方成员上限"},
	{Key: KeyPlan2Member, Type: settings.TypeInt, Default: "50", Description: "进阶档单立方成员上限"},
	{Key: KeyPlan3Member, Type: settings.TypeInt, Default: "0", Description: "高阶档单立方成员上限（0=不限）"},
}

// Plans 三档（settings 覆盖默认）。
func (s *Service) Plans(ctx context.Context) []contract.Plan {
	out := make([]contract.Plan, 0, 3)
	for _, t := range domain.DefaultTiers {
		out = append(out, contract.Plan{
			Level: t.Level, Name: t.Name, CubeLimit: t.CubeLimit, MemberLimit: t.MemberLimit, PriceFen: t.PriceFen,
		})
	}
	return out
}

// planTier levels settings 覆盖后的档位。
func (s *Service) planTier(ctx context.Context, level int16) domain.PlanTier {
	t := domain.Tier(level)
	if s.settings == nil {
		return t
	}
	read := func(key string) (int64, bool) {
		v, err := s.settings.Int(ctx, 1, key)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	switch level {
	case 2:
		if v, ok := read(KeyPlan2Price); ok {
			t.PriceFen = v
		}
		if v, ok := read(KeyPlan2Cube); ok {
			t.CubeLimit = int(v)
		}
		if v, ok := read(KeyPlan2Member); ok {
			t.MemberLimit = int(v)
		}
	case 3:
		if v, ok := read(KeyPlan3Price); ok {
			t.PriceFen = v
		}
		if v, ok := read(KeyPlan3Cube); ok {
			t.CubeLimit = int(v)
		}
		if v, ok := read(KeyPlan3Member); ok {
			t.MemberLimit = int(v)
		}
	default:
		if v, ok := read(KeyPlan1Cube); ok {
			t.CubeLimit = int(v)
		}
		if v, ok := read(KeyPlan1Member); ok {
			t.MemberLimit = int(v)
		}
	}
	return t
}

// PlanOf 立方当前档位定义。
func (s *Service) PlanOf(ctx context.Context, level int16) domain.PlanTier { return s.planTier(ctx, level) }

// ChangePlan 购买/升级订阅（生成 cube_plan 订单；履约在支付成功后）。
func (s *Service) ChangePlan(ctx context.Context, cubeID, userID int64, level int16, years int) (tradecontract.Order, error) {
	if !domain.ValidTier(level) {
		return tradecontract.Order{}, domain.ErrBadTier
	}
	if years <= 0 {
		years = 1
	}
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return tradecontract.Order{}, err
	}
	if c.LeaderID != userID {
		return tradecontract.Order{}, domain.ErrNoPermission
	}
	tier := s.planTier(ctx, level)
	if err := s.checkTierUpgrade(ctx, c, level); err != nil {
		return tradecontract.Order{}, err
	}
	amount := tier.PriceFen * int64(years)
	return s.trade.CreateOrder(ctx, userID, tradecontract.CreateOrderInput{
		BizType: "cube_plan", BizRef: strconv.FormatInt(cubeID, 10),
		AmountFen: amount, Subject: "猫立方订阅",
		PriceSnapshot: map[string]any{"cube_id": strconv.FormatInt(cubeID, 10), "level": level, "years": years},
		ExpireMinutes: 30,
	}, "")
}

// checkTierUpgrade 当前成员数不得超过目标档上限（降档约束）。
func (s *Service) checkTierUpgrade(ctx context.Context, c infra.Cube, level int16) error {
	tier := s.planTier(ctx, level)
	if tier.MemberLimit == 0 {
		return nil
	}
	n, err := s.repo.CountMembers(ctx, c.ID)
	if err != nil {
		return err
	}
	if n > tier.MemberLimit {
		return domain.ErrMemberLimit
	}
	return nil
}

// Fulfill 实现 trade.OrderFulfiller（biz_type=cube_plan）：开通/续期订阅（幂等）。
func (s *Service) Fulfill(ctx context.Context, order tradecontract.Order) error {
	if order.BizType != "cube_plan" {
		return nil
	}
	cubeID, err := strconv.ParseInt(order.BizRef, 10, 64)
	if err != nil {
		return err
	}
	var snap struct {
		Level int16 `json:"level"`
		Years int   `json:"years"`
	}
	_ = json.Unmarshal(order.PriceSnapshot, &snap)
	if snap.Years <= 0 {
		snap.Years = 1
	}
	base := time.Now().UTC()
	if c, err := s.repo.GetCube(ctx, cubeID); err == nil && c.PlanExpireAt != nil && c.PlanExpireAt.After(base) && c.PlanLevel == snap.Level {
		base = *c.PlanExpireAt // 同档续期顺延
	}
	expire := base.AddDate(snap.Years, 0, 0)
	return s.repo.SetPlan(ctx, cubeID, snap.Level, expire)
}

// ---- 分摊 ----

// InitiateApportion 猫王发起分摊（人数 N，不设价；每人=档价÷N）。
func (s *Service) InitiateApportion(ctx context.Context, cubeID, userID int64, level int16, n int, memberIDs []int64) (tradecontract.ApportionGroup, error) {
	if !domain.ValidTier(level) {
		return tradecontract.ApportionGroup{}, domain.ErrBadTier
	}
	c, err := s.repo.GetCube(ctx, cubeID)
	if err != nil {
		return tradecontract.ApportionGroup{}, err
	}
	if c.LeaderID != userID {
		return tradecontract.ApportionGroup{}, domain.ErrNoPermission
	}
	tier := s.planTier(ctx, level)
	if tier.PriceFen <= 0 {
		return tradecontract.ApportionGroup{}, domain.ErrBadTier
	}
	limit := tier.MemberLimit
	if limit == 0 {
		limit = 1 << 30
	}
	if n < 2 || n > limit {
		return tradecontract.ApportionGroup{}, domain.ErrInvalidInput
	}
	per := tier.PriceFen / int64(n)
	if per <= 0 {
		return tradecontract.ApportionGroup{}, domain.ErrBadTier
	}
	ids := make([]string, 0, len(memberIDs))
	for _, id := range memberIDs {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	return s.trade.CreateApportion(ctx, tradecontract.ApportionInput{
		CubePlanRef: strconv.FormatInt(cubeID, 10),
		InitiatorID: userID, PerFen: per, TargetN: n,
		Deadline: time.Now().UTC().AddDate(0, 0, 7),
		Snapshot: map[string]any{"cube_id": strconv.FormatInt(cubeID, 10), "level": level, "member_ids": ids},
	})
}

// PledgeApportion 认筹下单（认筹者无需已是成员；成团后自动成为共建成员）。
func (s *Service) PledgeApportion(ctx context.Context, cubeID, groupID, userID int64) (tradecontract.Order, error) {
	o, err := s.trade.PledgeOrder(ctx, groupID, userID, "")
	if errors.Is(err, tradecontract.ErrApportionClosed) {
		return tradecontract.Order{}, domain.ErrApportionClosed
	}
	return o, err
}

// ExitApportion 截止前退出退认筹。
func (s *Service) ExitApportion(ctx context.Context, cubeID, groupID, userID int64) (tradecontract.Refund, error) {
	return s.trade.ExitPledge(ctx, groupID, userID)
}

// ApportionOf 分摊组查询。
func (s *Service) ApportionOf(ctx context.Context, groupID int64) (tradecontract.ApportionGroup, error) {
	return s.trade.ApportionGroupOf(ctx, groupID)
}

// HandleApportionFormed 订阅 trade.ApportionFormed：开通订阅 + 成团成员入立方（标共建成员，不受上限）。
func (s *Service) HandleApportionFormed(ctx context.Context, payload []byte) error {
	var p struct {
		GroupID       int64           `json:"group_id,string"`
		CubePlanRef   string          `json:"cube_plan_ref"`
		InitiatorID   int64           `json:"initiator_id,string"`
		TargetN       int             `json:"target_n"`
		PledgedCount  int             `json:"pledged_count"`
		PriceSnapshot json.RawMessage `json:"price_snapshot"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	cubeID, err := strconv.ParseInt(p.CubePlanRef, 10, 64)
	if err != nil {
		return nil
	}
	var snap struct {
		Level     int16    `json:"level"`
		MemberIDs []string `json:"member_ids"`
	}
	_ = json.Unmarshal(p.PriceSnapshot, &snap)
	expire := time.Now().UTC().AddDate(1, 0, 0)
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		for _, idStr := range snap.MemberIDs {
			uid, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				continue
			}
			if err := s.repo.AddMember(ctx, tx, int64(ids.New()), cubeID, uid, contract.RoleCoBuild, 0); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.repo.SetPlan(ctx, cubeID, snap.Level, expire)
}

