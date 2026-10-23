// Package pay 支付适配器（微信支付）。开发态实现用于本地/CI 端到端，绝不用于生产。
package pay

import (
	"context"
	"encoding/json"
	"errors"

	"catcube/internal/ports"
)

// DevClient 开发态支付：预支付返回可预测凭据；回调按 body.sign 字段验签（ok=通过）。
type DevClient struct{}

func NewDevClient() *DevClient { return &DevClient{} }

// Prepay 返回开发态预支付单号。
func (d *DevClient) Prepay(_ context.Context, req ports.PayRequest) (ports.PayPrepay, error) {
	return ports.PayPrepay{PrepayID: "dev-prepay-" + req.OutTradeNo}, nil
}

// VerifyNotify 开发态验签：body 为 JSON {out_trade_no, amount_fen, sign}；sign != "ok" 视为验签失败。
func (d *DevClient) VerifyNotify(_ context.Context, _ map[string]string, body []byte) (string, int64, error) {
	var n struct {
		OutTradeNo string `json:"out_trade_no"`
		AmountFen  int64  `json:"amount_fen"`
		Sign       string `json:"sign"`
	}
	if err := json.Unmarshal(body, &n); err != nil {
		return "", 0, err
	}
	if n.Sign != "ok" {
		return "", 0, errors.New("pay: 验签失败")
	}
	if n.OutTradeNo == "" {
		return "", 0, errors.New("pay: 缺 out_trade_no")
	}
	return n.OutTradeNo, n.AmountFen, nil
}

// Refund 开发态退款直接成功。
func (d *DevClient) Refund(_ context.Context, _, _ string, _, _ int64) error { return nil }

var _ ports.Pay = (*DevClient)(nil)
