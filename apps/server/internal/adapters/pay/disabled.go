package pay

import (
	"context"
	"errors"

	"catcube/internal/ports"
)

// DisabledClient 生产未开通支付时的安全占位：所有支付动作明确失败，
// 绝不返回开发态可伪造回调（避免未配置商户号时被伪造支付）。
type DisabledClient struct{}

func NewDisabledClient() *DisabledClient { return &DisabledClient{} }

func (d *DisabledClient) Prepay(context.Context, ports.PayRequest) (ports.PayPrepay, error) {
	return ports.PayPrepay{}, errors.New("pay: 支付通道未开通")
}

func (d *DisabledClient) VerifyNotify(context.Context, map[string]string, []byte) (string, int64, error) {
	return "", 0, errors.New("pay: 支付通道未开通")
}

func (d *DisabledClient) Refund(context.Context, string, string, int64, int64) error {
	return errors.New("pay: 支付通道未开通")
}

var _ ports.Pay = (*DisabledClient)(nil)
