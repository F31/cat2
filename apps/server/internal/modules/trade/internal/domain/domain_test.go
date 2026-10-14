package domain

import (
	"context"
	"testing"

	"catcube/internal/kernel/statemachine"
)

func TestMachineValid(t *testing.T) {
	if err := statemachine.Validate(Machine); err != nil {
		t.Fatalf("状态机 #15 定义非法: %v", err)
	}
}

func TestLegalTransitions(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	cases := []struct{ from, event, actor, want string }{
		{StatePending, EventPay, "payment", StatePaid},
		{StatePending, EventTimeout, "system", StateClosed},
	}
	for _, c := range cases {
		got, err := eng.Apply(context.Background(), Machine, 1, c.from, c.event, c.actor)
		if err != nil || got != c.want {
			t.Fatalf("%s --%s(%s)--> 期望 %s 实际 %s err=%v", c.from, c.event, c.actor, c.want, got, err)
		}
	}
}

func TestIllegalTransitions(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	cases := []struct{ from, event, actor string }{
		{StatePaid, EventPay, "payment"},    // 已支付不可重复支付
		{StatePaid, EventTimeout, "system"}, // 已支付不可超时关闭
		{StateClosed, EventPay, "payment"},  // 终态无出边
		{StateClosed, EventTimeout, "system"},
	}
	for _, c := range cases {
		if _, err := eng.Apply(context.Background(), Machine, 1, c.from, c.event, c.actor); err == nil {
			t.Fatalf("非法转移应拒绝: %s --%s(%s)", c.from, c.event, c.actor)
		}
	}
}

func TestValidateCreate(t *testing.T) {
	if ValidateCreate("", 100) != ErrBizTypeRequired {
		t.Fatal("缺业务类型应拒绝")
	}
	if ValidateCreate("member", 0) != ErrAmountInvalid {
		t.Fatal("金额≤0 应拒绝")
	}
	if ValidateCreate("member", 100) != nil {
		t.Fatal("合法应通过")
	}
}

func TestValidateRefund(t *testing.T) {
	if ValidateRefund(0, 100) == nil || ValidateRefund(101, 100) == nil {
		t.Fatal("退款金额非法应拒绝")
	}
	if ValidateRefund(100, 100) != nil || ValidateRefund(50, 100) != nil {
		t.Fatal("合法退款金额应通过")
	}
}

func TestStateOf(t *testing.T) {
	if StateOf(0) != StatePending || StateOf(1) != StatePaid || StateOf(2) != StateClosed {
		t.Fatal("状态推导不符")
	}
}
