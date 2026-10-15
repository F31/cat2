// Package app trade 应用服务：下单/支付/回调/退款/履约/对账。
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/statemachine"
	"catcube/internal/modules/trade/contract"
	"catcube/internal/modules/trade/internal/domain"
	"catcube/internal/modules/trade/internal/infra"
	"catcube/internal/ports"
	"catcube/pkg/ids"
)

// Repo 应用层仓储能力。
type Repo interface {
	InsertOrderIdem(ctx context.Context, tx db.Tx, o infra.Order, idem string) error
	GetOrder(ctx context.Context, id int64) (infra.Order, error)
	GetOrderByIdem(ctx context.Context, idem string) (infra.Order, error)
	AdminListOrders(ctx context.Context, status int16, limit int) ([]infra.Order, error)
	ListExpired(ctx context.Context, limit int) ([]int64, error)
	CloseOrder(ctx context.Context, id int64) error
	MarkOrderPaid(ctx context.Context, tx db.Tx, id int64) error
	InsertPayment(ctx context.Context, tx db.Tx, p infra.Payment) error
	GetPaymentByOutTradeNo(ctx context.Context, no string) (infra.Payment, error)
	MarkPaymentPaid(ctx context.Context, tx db.Tx, id int64, rawNotify []byte) error
	LatestPaidPayment(ctx context.Context, orderID int64) (infra.Payment, error)
	InsertRefund(ctx context.Context, tx db.Tx, r infra.Refund) error
	GetRefundByIdem(ctx context.Context, idem string) (infra.Refund, error)
	MarkRefundStatus(ctx context.Context, tx db.Tx, id int64, status int16) error
	InsertFulfillment(ctx context.Context, tx db.Tx, orderID int64, bizType string) error
	PendingFulfillments(ctx context.Context, limit int) ([]infra.Fulfillment, error)
	UpdateFulfillment(ctx context.Context, orderID int64, ok bool, lastErr string) error
	UpsertRecon(ctx context.Context, date, channel string, orderCount int, amountFen int64, diff bool, detail []byte) error
	ResetFulfillment(ctx context.Context, orderID int64) (bool, error)
}

// Options 构造参数。
type Options struct {
	Repo Repo
	Pool *db.Pool
	Bus  *eventbus.Bus
	Pay  ports.Pay
}

// Service trade 应用服务。
type Service struct {
	repo Repo
	pool *db.Pool
	bus  *eventbus.Bus
	pay  ports.Pay

	mu         sync.RWMutex
	fulfillers map[string]contract.OrderFulfiller
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, pay: o.Pay, fulfillers: map[string]contract.OrderFulfiller{}}
}

// RegisterFulfiller 注册履约方。
func (s *Service) RegisterFulfiller(f contract.OrderFulfiller) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fulfillers[f.BizType()] = f
}

// CreateOrder 下单（幂等键去重）。
func (s *Service) CreateOrder(ctx context.Context, buyerID int64, in contract.CreateOrderInput, idemKey string) (contract.Order, error) {
	if err := domain.ValidateCreate(in.BizType, in.AmountFen); err != nil {
		return contract.Order{}, err
	}
	if idemKey != "" {
		if o, err := s.repo.GetOrderByIdem(ctx, idemKey); err == nil {
			return toOrder(o), nil
		} else if !errors.Is(err, infra.ErrNotFound) {
			return contract.Order{}, err
		}
	}
	expire := in.ExpireMinutes
	if expire <= 0 {
		expire = 30
	}
	o := infra.Order{
		ID: int64(ids.New()), BizType: in.BizType, BizRef: in.BizRef, BuyerID: buyerID,
		AmountFen: in.AmountFen, PriceSnapshot: infra.MarshalSnapshot(in.PriceSnapshot),
		Status: contract.StatusPending, PayExpireAt: time.Now().UTC().Add(time.Duration(expire) * time.Minute),
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.InsertOrderIdem(ctx, tx, o, idemKey); err != nil {
			return err
		}
		return s.emit(ctx, tx, contract.EventOrderCreated, o)
	}); err != nil {
		if idemKey != "" {
			if existing, e := s.repo.GetOrderByIdem(ctx, idemKey); e == nil {
				return toOrder(existing), nil
			}
		}
		return contract.Order{}, err
	}
	return toOrder(o), nil
}

// Pay 发起支付（一单多笔，out_trade_no 唯一）。
func (s *Service) Pay(ctx context.Context, buyerID, orderID int64) (contract.PayParams, error) {
	o, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return contract.PayParams{}, mapNotFound(err)
	}
	if o.BuyerID != buyerID {
		return contract.PayParams{}, domain.ErrForbidden
	}
	if o.Status != contract.StatusPending {
		return contract.PayParams{}, domain.ErrNotPayable
	}
	out := fmt.Sprintf("CC%d%d", o.ID, ids.New())
	p := infra.Payment{ID: int64(ids.New()), OrderID: o.ID, OutTradeNo: out, Channel: "wechat", AmountFen: o.AmountFen}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.InsertPayment(ctx, tx, p)
	}); err != nil {
		return contract.PayParams{}, err
	}
	prepay := "dev-prepay-" + out
	if s.pay != nil {
		prepay, err = s.pay.Prepay(ctx, ports.PayRequest{OutTradeNo: out, AmountFen: o.AmountFen, Subject: "猫平方订单"})
		if err != nil {
			return contract.PayParams{}, err
		}
	}
	return contract.PayParams{OrderID: o.ID, OutTradeNo: out, PrepayID: prepay, AmountFen: o.AmountFen}, nil
}

// HandleNotify 支付回调：验签 → 幂等 → 状态机 → 履约待办 + OrderPaid。
func (s *Service) HandleNotify(ctx context.Context, headers map[string]string, body []byte) (contract.NotifyResult, error) {
	if s.pay == nil {
		return contract.NotifyResult{}, domain.ErrNotifyInvalid
	}
	outNo, amount, err := s.pay.VerifyNotify(ctx, headers, body)
	if err != nil {
		return contract.NotifyResult{}, domain.ErrNotifyInvalid
	}
	p, err := s.repo.GetPaymentByOutTradeNo(ctx, outNo)
	if err != nil {
		return contract.NotifyResult{}, mapNotFound(err)
	}
	o, err := s.repo.GetOrder(ctx, p.OrderID)
	if err != nil {
		return contract.NotifyResult{}, mapNotFound(err)
	}
	// 幂等：支付单已支付 → 不重复置状态、不重复发事件
	if p.Status == contract.StatusPaid {
		return contract.NotifyResult{OrderID: o.ID, AlreadyPaid: true}, nil
	}
	if amount != 0 && amount != p.AmountFen {
		return contract.NotifyResult{}, fmt.Errorf("trade: 回调金额不一致")
	}
	if o.Status != contract.StatusPending {
		// 订单已关闭/已支付：仅补齐支付单状态，不再发 OrderPaid
		return contract.NotifyResult{OrderID: o.ID, AlreadyPaid: o.Status == contract.StatusPaid}, nil
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.MarkPaymentPaid(ctx, tx, p.ID, body); err != nil {
			return err
		}
		if _, err := statemachine.NewEngine(nil).Apply(ctx, domain.Machine, o.ID, domain.StatePending, domain.EventPay, "payment"); err != nil {
			return err
		}
		if err := s.repo.MarkOrderPaid(ctx, tx, o.ID); err != nil {
			return err
		}
		if err := s.repo.InsertFulfillment(ctx, tx, o.ID, o.BizType); err != nil {
			return err
		}
		return s.emit(ctx, tx, contract.EventOrderPaid, o)
	})
	if err != nil {
		return contract.NotifyResult{}, err
	}
	return contract.NotifyResult{OrderID: o.ID, AlreadyPaid: false}, nil
}

// Refund 后台发起退款（幂等键=payment_id+reason，原路退回）。
func (s *Service) Refund(ctx context.Context, operatorID, orderID, amountFen int64, reason string) (contract.Refund, error) {
	if strings.TrimSpace(reason) == "" {
		return contract.Refund{}, domain.ErrRefundAmount
	}
	o, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return contract.Refund{}, mapNotFound(err)
	}
	pay, err := s.repo.LatestPaidPayment(ctx, orderID)
	if err != nil {
		return contract.Refund{}, domain.ErrPaymentNotFound
	}
	if amountFen == 0 {
		amountFen = pay.AmountFen
	}
	if err := domain.ValidateRefund(amountFen, pay.AmountFen); err != nil {
		return contract.Refund{}, err
	}
	idem := fmt.Sprintf("%d:%s", pay.ID, reason)
	if existing, err := s.repo.GetRefundByIdem(ctx, idem); err == nil {
		return toRefund(existing), nil
	}
	r := infra.Refund{ID: int64(ids.New()), PaymentID: pay.ID, Reason: reason, AmountFen: amountFen,
		Status: 0, RefundNo: fmt.Sprintf("RF%d", ids.New()), OperatorID: operatorID, IdemKey: idem}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.InsertRefund(ctx, tx, r)
	}); err != nil {
		if existing, e := s.repo.GetRefundByIdem(ctx, idem); e == nil {
			return toRefund(existing), nil
		}
		return contract.Refund{}, err
	}
	status := int16(1)
	if s.pay != nil {
		if err := s.pay.Refund(ctx, pay.OutTradeNo, r.RefundNo, amountFen); err != nil {
			status = 2
		}
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.MarkRefundStatus(ctx, tx, r.ID, status)
	}); err != nil {
		return contract.Refund{}, err
	}
	r.Status = status
	if status == 1 {
		_ = s.pool.InTx(ctx, func(tx db.Tx) error {
			if s.bus == nil {
				return nil
			}
			return s.bus.Publish(ctx, tx, contract.NewRefundSucceeded(ids.New().String(), contract.RefundPayload{
				RefundID: r.ID, OrderID: o.ID, BuyerID: o.BuyerID, AmountFen: amountFen,
			}))
		})
	}
	return toRefund(r), nil
}

// GetOrder 查询订单。
func (s *Service) GetOrder(ctx context.Context, orderID int64) (contract.Order, error) {
	o, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return contract.Order{}, mapNotFound(err)
	}
	return toOrder(o), nil
}

// AdminListOrders 后台订单列表。
func (s *Service) AdminListOrders(ctx context.Context, status int16, limit int) ([]contract.Order, error) {
	rows, err := s.repo.AdminListOrders(ctx, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Order, 0, len(rows))
	for _, o := range rows {
		out = append(out, toOrder(o))
	}
	return out, nil
}

// CloseExpired 关闭超时未支付订单（SchedulerJob）。
func (s *Service) CloseExpired(ctx context.Context) error {
	orders, err := s.repo.ListExpired(ctx, 100)
	if err != nil {
		return err
	}
	for _, id := range orders {
		o, err := s.repo.GetOrder(ctx, id)
		if err != nil {
			continue
		}
		if _, err := statemachine.NewEngine(nil).Apply(ctx, domain.Machine, id, domain.StatePending, domain.EventTimeout, "system"); err != nil {
			continue
		}
		if err := s.repo.CloseOrder(ctx, id); err != nil {
			continue
		}
		_ = s.pool.InTx(ctx, func(tx db.Tx) error {
			return s.emit(ctx, tx, contract.EventOrderClosed, o)
		})
	}
	return nil
}

// RunFulfillment 履约待办：按 biz_type 调用注册的履约方，失败重试（≥8 死信）。
func (s *Service) RunFulfillment(ctx context.Context) (int, error) {
	list, err := s.repo.PendingFulfillments(ctx, 50)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, f := range list {
		s.mu.RLock()
		ful, ok := s.fulfillers[f.BizType]
		s.mu.RUnlock()
		if !ok {
			_ = s.repo.UpdateFulfillment(ctx, f.OrderID, false, "未注册履约方:"+f.BizType)
			continue
		}
		o, err := s.repo.GetOrder(ctx, f.OrderID)
		if err != nil {
			continue
		}
		if err := ful.Fulfill(ctx, toOrder(o)); err != nil {
			_ = s.repo.UpdateFulfillment(ctx, f.OrderID, false, err.Error())
			continue
		}
		_ = s.repo.UpdateFulfillment(ctx, f.OrderID, true, "")
		done++
	}
	return done, nil
}

// RetryFulfillment 后台手动补偿：死信履约单重置重试。
func (s *Service) RetryFulfillment(ctx context.Context, orderID int64) (bool, error) {
	return s.repo.ResetFulfillment(ctx, orderID)
}

// RunRecon 每日对账（dev 以订单侧汇总为准，差异需对接渠道账单）。
func (s *Service) RunRecon(ctx context.Context) error {
	date := time.Now().UTC().Format("2006-01-02")
	var cnt int
	var amt int64
	if err := s.pool.QueryRow(ctx,
		`SELECT count(1), COALESCE(sum(amount_fen),0) FROM trd_order WHERE status=1 AND paid_at::date=$1::date`, date).Scan(&cnt, &amt); err != nil {
		return err
	}
	return s.repo.UpsertRecon(ctx, date, "wechat", cnt, amt, false, nil)
}

// ---- 内部 ----

func (s *Service) emit(ctx context.Context, tx db.Tx, topic string, o infra.Order) error {
	if s.bus == nil {
		return nil
	}
	return s.bus.Publish(ctx, tx, contract.NewOrderEvent(ids.New().String(), topic, contract.OrderPayload{
		OrderID: o.ID, BizType: o.BizType, BizRef: o.BizRef, BuyerID: o.BuyerID,
		AmountFen: o.AmountFen, PriceSnapshot: o.PriceSnapshot,
	}))
}

func mapNotFound(err error) error {
	if errors.Is(err, infra.ErrNotFound) {
		return domain.ErrOrderNotFound
	}
	return err
}

func toOrder(o infra.Order) contract.Order {
	return contract.Order{
		ID: o.ID, BizType: o.BizType, BizRef: o.BizRef, BuyerID: o.BuyerID, AmountFen: o.AmountFen,
		PriceSnapshot: o.PriceSnapshot, Status: o.Status, PayExpireAt: o.PayExpireAt, PaidAt: o.PaidAt, ClosedAt: o.ClosedAt,
	}
}

func toRefund(r infra.Refund) contract.Refund {
	return contract.Refund{ID: r.ID, PaymentID: r.PaymentID, AmountFen: r.AmountFen, Status: r.Status}
}
