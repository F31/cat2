package statemachine

import (
	"context"
	"errors"
	"testing"
)

// 表驱动测试框架示范：PRD 11.1 #15 订单状态机的最小模型。
var orderDef = Def{
	Name:   "trade.order",
	States: []string{"pending", "paid", "closed"},
	Terminal: []string{"closed"},
	Transitions: []Transition{
		{From: "pending", Event: "pay", To: "paid", Actor: "*"},
		{From: "pending", Event: "timeout", To: "closed", Actor: "system"},
		{From: "paid", Event: "refund", To: "closed", Actor: "admin"},
	},
}

func TestLegalTransitions(t *testing.T) {
	eng := NewEngine(nil)
	cases := []struct{ from, event, actor, want string }{
		{"pending", "pay", "user", "paid"},
		{"pending", "timeout", "system", "closed"},
		{"paid", "refund", "admin", "closed"},
	}
	for _, c := range cases {
		got, err := eng.Apply(context.Background(), orderDef, 1, c.from, c.event, c.actor)
		if err != nil || got != c.want {
			t.Fatalf("%s --%s--> 期望 %s，实际 %s err=%v", c.from, c.event, c.want, got, err)
		}
	}
}

func TestIllegalTransitionRejected(t *testing.T) {
	eng := NewEngine(nil)
	cases := []struct{ from, event, actor string }{
		{"closed", "pay", "user"},     // 终态无出边
		{"paid", "timeout", "system"}, // 已支付不可超时关闭
		{"paid", "refund", "user"},    // 退款只能后台财务（admin）
	}
	for _, c := range cases {
		if _, err := eng.Apply(context.Background(), orderDef, 1, c.from, c.event, c.actor); err == nil {
			t.Fatalf("非法转移应被拒绝: %s --%s(%s)", c.from, c.event, c.actor)
		}
	}
}

func TestGuard(t *testing.T) {
	def := Def{
		Name:   "test",
		States: []string{"a", "b"},
		Transitions: []Transition{
			{From: "a", Event: "go", To: "b", Actor: "*", Guard: func(_ context.Context, _ string, _ int64) error {
				return errors.New("守卫拒绝")
			}},
		},
	}
	if _, err := NewEngine(nil).Apply(context.Background(), def, 1, "a", "go", "user"); err == nil {
		t.Fatal("守卫拒绝应阻断转移")
	}
}

func TestLogEmitted(t *testing.T) {
	var logs []Log
	eng := NewEngine(func(l Log) { logs = append(logs, l) })
	if _, err := eng.Apply(context.Background(), orderDef, 42, "pending", "pay", "user"); err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].To != "paid" || logs[0].EntityID != 42 {
		t.Fatalf("状态日志缺失: %+v", logs)
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(orderDef); err != nil {
		t.Fatal(err)
	}
	bad := Def{Name: "x", States: []string{"a"}, Transitions: []Transition{{From: "a", Event: "e", To: "ghost"}}}
	if err := Validate(bad); err == nil {
		t.Fatal("引用未声明状态应报错")
	}
}
