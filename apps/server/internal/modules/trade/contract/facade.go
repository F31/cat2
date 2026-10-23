// Package contract trade 对外契约：订单、支付、退款、履约注册表。
package contract

import (
	"context"
	"encoding/json"
	"errors"
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
	// 微信支付调起参数（真实支付通道下发；开发态为空）。
	TimeStamp string `json:"timeStamp,omitempty"`
	NonceStr  string `json:"nonceStr,omitempty"`
	Package   string `json:"package,omitempty"`
	SignType  string `json:"signType,omitempty"`
	PaySign   string `json:"paySign,omitempty"`
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
	// CreateApportion 发起分摊（猫王设人数不设价；每人=per_fen）。
	CreateApportion(ctx context.Context, in ApportionInput) (ApportionGroup, error)
	// PledgeOrder 认筹下单（生成认筹订单；支付成功后计数强一致成团）。
	PledgeOrder(ctx context.Context, groupID, buyerID int64, idempotencyKey string) (Order, error)
	// ExitPledge 截止前退出（原路退回）。
	ExitPledge(ctx context.Context, groupID, buyerID int64) (Refund, error)
	// ApportionGroupOf 取分摊组。
	ApportionGroupOf(ctx context.Context, groupID int64) (ApportionGroup, error)
}

// ---- 分摊认筹（强一致在 trade 事务内）----

// 分摊组状态。
const (
	ApportionActive  int16 = 0 // 进行中
	ApportionFormed  int16 = 1 // 已成团
	ApportionFailed  int16 = 2 // 未成团
	ApportionSettled int16 = 3 // 已终结（退款全部完成）
)

// ErrApportionClosed 分摊已结束/已成团（供调用方判别）。
var ErrApportionClosed = errors.New("trade: 分摊已结束/已成团")

// ApportionBizType 分摊订单业务类型。
const ApportionBizType = "cube_apportion"

// ApportionInput 发起分摊入参（不设价，每人=per_fen）。
type ApportionInput struct {
	CubePlanRef string
	InitiatorID int64
	PerFen      int64
	TargetN     int
	Deadline    time.Time
	Snapshot    map[string]any
}

// ApportionGroup 分摊组。
type ApportionGroup struct {
	ID           int64     `json:"id,string"`
	CubePlanRef  string    `json:"cube_plan_ref"`
	InitiatorID  int64     `json:"initiator_id,string"`
	TotalFen     int64     `json:"total_fen"`
	PerFen       int64     `json:"per_fen"`
	TargetN      int       `json:"target_n"`
	PledgedCount int       `json:"pledged_count"`
	Deadline     time.Time `json:"deadline"`
	Status       int16     `json:"status"`
}
