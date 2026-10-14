// Package contract trade 领域事件（附录 C）。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// 事件 Topic。
const (
	EventOrderCreated    = "trade.OrderCreated.v1"
	EventOrderPaid       = "trade.OrderPaid.v1"
	EventOrderClosed     = "trade.OrderClosed.v1"
	EventRefundSucceeded = "trade.RefundSucceeded.v1"
)

// OrderPayload 订单事件载荷（不含支付凭据）。
type OrderPayload struct {
	OrderID       int64           `json:"order_id,string"`
	BizType       string          `json:"biz_type"`
	BizRef        string          `json:"biz_ref,omitempty"`
	BuyerID       int64           `json:"buyer_id,string"`
	AmountFen     int64           `json:"amount_fen"`
	PriceSnapshot json.RawMessage `json:"price_snapshot,omitempty"`
}

// RefundPayload 退款成功载荷。
type RefundPayload struct {
	RefundID  int64 `json:"refund_id,string"`
	OrderID   int64 `json:"order_id,string"`
	BuyerID   int64 `json:"buyer_id,string"`
	AmountFen int64 `json:"amount_fen"`
}

func newEvent(id, topic string, p any) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: topic, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}

func NewOrderEvent(id, topic string, p OrderPayload) eventbus.Event { return newEvent(id, topic, p) }
func NewRefundSucceeded(id string, p RefundPayload) eventbus.Event {
	return newEvent(id, EventRefundSucceeded, p)
}
