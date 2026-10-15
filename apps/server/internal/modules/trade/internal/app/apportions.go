// apportions.go trade 分摊认筹：成团强一致（trade 事务内锁行计数）、到期退款、退出退款。
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/modules/trade/contract"
	"catcube/internal/modules/trade/internal/domain"
	"catcube/internal/modules/trade/internal/infra"
	"catcube/pkg/ids"
)

// CreateApportion 发起分摊（不设价；每人=per_fen，2≤N≤上限由调用方校验）。
func (s *Service) CreateApportion(ctx context.Context, in contract.ApportionInput) (contract.ApportionGroup, error) {
	if in.TargetN < 2 || in.PerFen <= 0 || in.Deadline.IsZero() {
		return contract.ApportionGroup{}, domain.ErrApportionInvalid
	}
	snap, _ := json.Marshal(in.Snapshot)
	g := infra.ApportionGroup{
		ID: int64(ids.New()), CubePlanRef: in.CubePlanRef, InitiatorID: in.InitiatorID,
		TotalFen: in.PerFen * int64(in.TargetN), PerFen: in.PerFen, TargetN: in.TargetN,
		Deadline: in.Deadline, Snapshot: snap,
	}
	if err := s.repo.InsertApportion(ctx, s.pool, g); err != nil {
		return contract.ApportionGroup{}, err
	}
	return toApportion(g), nil
}

// PledgeOrder 认筹下单（金额=每人分摊额；支付成功后计数成团）。
func (s *Service) PledgeOrder(ctx context.Context, groupID, buyerID int64, idemKey string) (contract.Order, error) {
	g, err := s.repo.GetApportion(ctx, groupID)
	if err != nil {
		return contract.Order{}, mapNotFound(err)
	}
	if g.Status != contract.ApportionActive {
		return contract.Order{}, contract.ErrApportionClosed
	}
	minutes := int(time.Until(g.Deadline).Minutes())
	if minutes <= 0 {
		return contract.Order{}, contract.ErrApportionClosed
	}
	return s.CreateOrder(ctx, buyerID, contract.CreateOrderInput{
		BizType: contract.ApportionBizType, BizRef: strconv.FormatInt(groupID, 10),
		AmountFen: g.PerFen, Subject: "猫立方分摊认筹",
		PriceSnapshot: map[string]any{"group_id": strconv.FormatInt(groupID, 10), "per_fen": g.PerFen},
		ExpireMinutes: minutes,
	}, idemKey)
}

// ExitPledge 截止前退出（原路退款，幂等）。
func (s *Service) ExitPledge(ctx context.Context, groupID, buyerID int64) (contract.Refund, error) {
	g, err := s.repo.GetApportion(ctx, groupID)
	if err != nil {
		return contract.Refund{}, mapNotFound(err)
	}
	if g.Status != contract.ApportionActive {
		return contract.Refund{}, contract.ErrApportionClosed
	}
	p, err := s.repo.GetPledge(ctx, groupID, buyerID)
	if err != nil {
		return contract.Refund{}, mapNotFound(err)
	}
	if p.Status == 1 {
		return contract.Refund{}, nil // 已退
	}
	pay, err := s.repo.GetPaymentByID(ctx, p.PaymentID)
	if err != nil {
		return contract.Refund{}, mapNotFound(err)
	}
	r, err := s.Refund(ctx, 0, pay.OrderID, p.AmountFen, fmt.Sprintf("apportion:%d:exit", groupID))
	if err != nil {
		return contract.Refund{}, err
	}
	if err := s.repo.MarkPledgeRefunded(ctx, s.pool, p.ID); err != nil {
		return contract.Refund{}, err
	}
	return r, nil
}

// ApportionGroupOf 取分摊组。
func (s *Service) ApportionGroupOf(ctx context.Context, groupID int64) (contract.ApportionGroup, error) {
	g, err := s.repo.GetApportion(ctx, groupID)
	if err != nil {
		return contract.ApportionGroup{}, mapNotFound(err)
	}
	return toApportion(g), nil
}

// settleApportion 支付成功后的认筹结算（在同一 tx 内锁行，强一致杜绝超额）。
// 返回 over=true 表示应退（超额/已关闭），formed=true 表示本次成团。
func (s *Service) settleApportion(ctx context.Context, tx db.Tx, o infra.Order, payment infra.Payment) (over, formed bool, err error) {
	groupID, convErr := strconv.ParseInt(o.BizRef, 10, 64)
	if convErr != nil {
		return false, false, nil
	}
	g, err := s.repo.LockApportion(ctx, tx, groupID)
	if err != nil {
		if errors.Is(err, infra.ErrNotFound) {
			return true, false, nil
		}
		return false, false, err
	}
	if g.Status != contract.ApportionActive || g.PledgedCount >= g.TargetN {
		return true, false, nil
	}
	inserted, err := s.repo.InsertPledge(ctx, tx, int64(ids.New()), groupID, o.BuyerID, o.AmountFen, payment.ID)
	if err != nil {
		return false, false, err
	}
	if !inserted {
		return false, false, nil // 重复认筹：已计入，不重复计数
	}
	if err := s.repo.IncrPledged(ctx, tx, groupID); err != nil {
		return false, false, err
	}
	if g.PledgedCount+1 >= g.TargetN {
		if err := s.repo.SetApportionStatus(ctx, tx, groupID, contract.ApportionFormed); err != nil {
			return false, false, err
		}
		return false, true, nil
	}
	return false, false, nil
}

// ExpireApportions 到期未成团：逐笔幂等全额退回，全部完成才终结（SchedulerJob）。
func (s *Service) ExpireApportions(ctx context.Context) error {
	groups, err := s.repo.ListExpiredApportions(ctx, 200)
	if err != nil {
		return err
	}
	for _, groupID := range groups {
		g, err := s.repo.GetApportion(ctx, groupID)
		if err != nil {
			continue
		}
		if err := s.repo.SetApportionStatus(ctx, s.pool, groupID, contract.ApportionFailed); err != nil {
			continue
		}
		// 逐笔退款（幂等键含认筹 ID，重复执行返回首次结果）
		pledges, err := s.repo.ListPendingRefunds(ctx, groupID)
		if err != nil {
			continue
		}
		for _, p := range pledges {
			pay, err := s.repo.GetPaymentByID(ctx, p.PaymentID)
			if err != nil {
				continue
			}
			if _, err := s.Refund(ctx, 0, pay.OrderID, p.AmountFen, fmt.Sprintf("apportion:%d:pledge:%d", groupID, p.ID)); err != nil {
				slog.Warn("trade 未成团退款失败", "group", groupID, "pledge", p.ID, "err", err)
				continue
			}
			_ = s.repo.MarkPledgeRefunded(ctx, s.pool, p.ID)
		}
		// 全部退款完成才终结
		left, err := s.repo.CountUnsettledPledges(ctx, groupID)
		if err != nil || left > 0 {
			continue
		}
		if err := s.repo.SetApportionStatus(ctx, s.pool, groupID, contract.ApportionSettled); err != nil {
			continue
		}
		_ = s.pool.InTx(ctx, func(tx db.Tx) error {
			if s.bus == nil {
				return nil
			}
			return s.bus.Publish(ctx, tx, contract.NewApportionFailed(ids.New().String(), contract.ApportionPayload{
				GroupID: g.ID, CubePlanRef: g.CubePlanRef, InitiatorID: g.InitiatorID,
				TargetN: g.TargetN, PledgedCount: g.PledgedCount, PriceSnapshot: g.Snapshot,
			}))
		})
	}
	return nil
}

// emitApportionFormed 成团事件（在同事务内 emit）。
func (s *Service) emitApportionFormed(ctx context.Context, tx db.Tx, groupID int64) error {
	g, err := s.repo.GetApportion(ctx, groupID)
	if err != nil {
		return err
	}
	if s.bus == nil {
		return nil
	}
	return s.bus.Publish(ctx, tx, contract.NewApportionFormed(ids.New().String(), contract.ApportionPayload{
		GroupID: g.ID, CubePlanRef: g.CubePlanRef, InitiatorID: g.InitiatorID,
		TargetN: g.TargetN, PledgedCount: g.PledgedCount, PriceSnapshot: g.Snapshot,
	}))
}

func toApportion(g infra.ApportionGroup) contract.ApportionGroup {
	return contract.ApportionGroup{ID: g.ID, CubePlanRef: g.CubePlanRef, InitiatorID: g.InitiatorID,
		TotalFen: g.TotalFen, PerFen: g.PerFen, TargetN: g.TargetN, PledgedCount: g.PledgedCount,
		Deadline: g.Deadline, Status: g.Status}
}
