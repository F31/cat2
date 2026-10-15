// Package domain trade 领域规则：订单状态机 #15、金额与退款校验（纯函数）。
package domain

import (
	"errors"

	"catcube/internal/kernel/statemachine"
)

// 领域错误（映射到错误码 08xxxx）。
var (
	ErrOrderNotFound     = errors.New("trade: 订单不存在")
	ErrForbidden         = errors.New("trade: 无权操作该订单")
	ErrAmountInvalid     = errors.New("trade: 金额非法")
	ErrBizTypeRequired   = errors.New("trade: 业务类型必填")
	ErrNotPayable        = errors.New("trade: 订单当前状态不可支付")
	ErrPaymentNotFound   = errors.New("trade: 支付单不存在")
	ErrNotifyInvalid     = errors.New("trade: 支付回调验签失败")
	ErrRefundAmount      = errors.New("trade: 退款金额非法")
	ErrAlreadyFulfilled  = errors.New("trade: 订单已履约")
	ErrFulfillNoProvider = errors.New("trade: 未注册履约方")
	ErrApportionClosed   = errors.New("trade: 分摊已结束/已成团")
	ErrApportionInvalid  = errors.New("trade: 分摊参数非法")
)

// 状态机 #15 状态。
const (
	StatePending = "pending_pay" // 待支付
	StatePaid    = "paid"        // 已支付
	StateClosed  = "closed"      // 已关闭
)

// 状态机 #15 事件。
const (
	EventPay     = "pay"
	EventTimeout = "timeout"
)

// Machine 订单状态机 #15：待支付→已支付/已关闭；超时关闭由 SchedulerJob 触发。
var Machine = statemachine.Def{
	Name:     "trade.order",
	States:   []string{StatePending, StatePaid, StateClosed},
	Terminal: []string{StateClosed},
	Transitions: []statemachine.Transition{
		{From: StatePending, Event: EventPay, To: StatePaid, Actor: "*"},            // 支付回调
		{From: StatePending, Event: EventTimeout, To: StateClosed, Actor: "system"}, // 超时关闭
	},
}

// ValidateCreate 下单参数校验。
func ValidateCreate(bizType string, amountFen int64) error {
	if bizType == "" {
		return ErrBizTypeRequired
	}
	if amountFen <= 0 {
		return ErrAmountInvalid
	}
	return nil
}

// ValidateRefund 退款金额校验（不得超过支付金额）。
func ValidateRefund(amountFen, paidFen int64) error {
	if amountFen <= 0 || amountFen > paidFen {
		return ErrRefundAmount
	}
	return nil
}

// StateOf 由订单状态推导状态机状态。
func StateOf(status int16) string {
	switch status {
	case 1:
		return StatePaid
	case 2:
		return StateClosed
	default:
		return StatePending
	}
}
