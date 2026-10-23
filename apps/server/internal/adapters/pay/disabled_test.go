package pay

import (
	"context"
	"testing"

	"catcube/internal/ports"
)

func TestDisabledClientFailsClosed(t *testing.T) {
	c := NewDisabledClient()
	if _, err := c.Prepay(context.Background(), ports.PayRequest{OutTradeNo: "X", AmountFen: 1}); err == nil {
		t.Fatal("禁用支付时下单应失败")
	}
	if _, _, err := c.VerifyNotify(context.Background(), nil, []byte(`{"sign":"ok"}`)); err == nil {
		t.Fatal("禁用支付时回调应失败")
	}
	if err := c.Refund(context.Background(), "X", "R", 1, 1); err == nil {
		t.Fatal("禁用支付时退款应失败")
	}
}
