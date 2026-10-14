// Package contract trade 对外契约：订单、支付、退款、履约注册表。
package contract

import (
	"context"
	"encoding/json"
	"time"
)

// 订单状态（状态机 #15：待支付→已支付/已关闭）。
const (
	StatusPending int16 = 0 // 待支付
	StatusPaid    int16 = 1 // 已支付
	StatusClosed  int16 = 2 // 已关闭
)

// Order 订单。
type Order struct {
	ID            int64           `json:"id,string"`
	BizType       string          `json:"biz_type"`
	BizRef        string          `json:"biz_ref,omitempty"`
	BuyerID       int64           `json:"buyer_id,string"`
	AmountFen     int64           `json:"amount_fen"`
	PriceSnapshot json.RawMessage `json:"price_snapshot"`
	Status        int16           `json:"status"`
	PayExpireAt   time.Time       `json:"pay_expire_at"`
	PaidAt        *time.Time      `json:"paid_at,omitempty"`
	ClosedAt      *time.Time      `json:"closed_at,omitempty"`
}

// CreateOrderInput 下单入参（金额由业务模块计算并给快照，trade 不重新计价）。
type CreateOrderInput struct {
	BizType       string         `json:"biz_type"`
	BizRef        string         `json:"biz_ref"`
	BuyerID       int64          `json:"buyer_id"`
	AmountFen     int64          `json:"amount_fen"`
	PriceSnapshot map[string]any `json:"price_snapshot"`
	Subject       string         `json:"subject"`
	ExpireMinutes int            `json:"expire_minutes"`
}

// PayParams 发起支付返回（预支付参数）。
type PayParams struct {
	OrderID    int64  `json:"order_id,string"`
	OutTradeNo string `json:"out_trade_no"`
	PrepayID   string `json:"prepay_id"`
	AmountFen  int64  `json:"amount_fen"`
}

// NotifyResult 支付回调处理结果。
type NotifyResult struct {
	OrderID     int64 `json:"order_id,string"`
	AlreadyPaid bool  `json:"already_paid"`
}

// Refund 退款记录。
type Refund struct {
	ID        int64     `json:"id,string"`
	PaymentID int64     `json:"payment_id,string"`
	AmountFen int64     `json:"amount_fen"`
	Status    int16     `json:"status"`
	RefundNo  string    `json:"refund_no"`
	CreatedAt time.Time `json:"created_at"`
}

// OrderFulfiller 履约方（扩展点：按 biz_type 履约，以订单 ID 幂等）。
type OrderFulfiller interface {
	BizType() string
	Fulfill(ctx context.Context, order Order) error
}

// Facade trade 对外接口（只管钱）。
type Facade interface {
	// CreateOrder 下单（幂等键去重）。
	CreateOrder(ctx context.Context, buyerID int64, in CreateOrderInput, idempotencyKey string) (Order, error)
	// Pay 发起支付（一单可多笔，out_trade_no 唯一）。
	Pay(ctx context.Context, buyerID, orderID int64) (PayParams, error)
	// GetOrder 查询订单（含归属校验由调用方决定）。
	GetOrder(ctx context.Context, orderID int64) (Order, error)
	// Refund 后台发起退款（原路退回，幂等键=payment_id+reason）。
	Refund(ctx context.Context, operatorID, orderID int64, amountFen int64, reason string) (Refund, error)
	// RegisterFulfiller 注册履约方（高阶模块启动时注册）。
	RegisterFulfiller(f OrderFulfiller)
}
