package domain

import (
	"context"
	"testing"
	"time"

	"catcube/internal/kernel/statemachine"
)

func TestValidLevel(t *testing.T) {
	if !ValidLevel(1) || !ValidLevel(3) {
		t.Fatal("VIP/企业版应有效")
	}
	if ValidLevel(2) {
		t.Fatal("SVIP 预留不上架")
	}
	if ValidLevel(0) || ValidLevel(9) {
		t.Fatal("非法等级应为无效")
	}
}

func TestDiscountCoeff(t *testing.T) {
	cases := map[int]int{1: 10000, 2: 10000, 3: 9000, 4: 10000, 5: 8000}
	for years, want := range cases {
		if got := DiscountCoeff(years, 9000, 8000); got != want {
			t.Fatalf("%d 年系数应 %d，得 %d", years, want, got)
		}
	}
}

func TestQuote(t *testing.T) {
	if o, total := Quote(9900, 1, 10000); o != 9900 || total != 9900 {
		t.Fatalf("1 年报价不符: %d %d", o, total)
	}
	if o, total := Quote(9900, 3, 9000); o != 29700 || total != 26730 {
		t.Fatalf("3 年报价不符: %d %d", o, total)
	}
	if o, total := Quote(9900, 5, 8000); o != 49500 || total != 39600 {
		t.Fatalf("5 年报价不符: %d %d", o, total)
	}
}

func TestRefundAmount(t *testing.T) {
	now := time.Now().UTC()
	// 刚开通即退 → 全额（一年）
	if got := RefundAmount(9900, 1, 10000, now.Add(365*24*time.Hour), now); got != 9900 {
		t.Fatalf("刚开通退款应 9900，得 %d", got)
	}
	// 剩余半年（不满 1 年）→ 0 个完整年
	if got := RefundAmount(9900, 1, 10000, now.Add(180*24*time.Hour), now); got != 0 {
		t.Fatalf("半年剩余应为 0，得 %d", got)
	}
	// 5 年、已用 1 年 → 剩余 4 年 × 0.8
	if got := RefundAmount(9900, 5, 8000, now.Add(4*365*24*time.Hour), now); got != 9900*4*8000/10000 {
		t.Fatalf("剩余 4 年退款不符，得 %d", got)
	}
	// 已过期 → 0
	if got := RefundAmount(9900, 1, 10000, now.Add(-time.Hour), now); got != 0 {
		t.Fatalf("已过期应 0，得 %d", got)
	}
}

func TestMachineValid(t *testing.T) {
	if err := statemachine.Validate(Machine); err != nil {
		t.Fatalf("状态机 #12 定义非法: %v", err)
	}
	if StateOf(1, true) != StateActive || StateOf(2, true) != StateExpired || StateOf(0, false) != StateNone {
		t.Fatal("状态推导错误")
	}
}

// 表驱动：状态机 #12 合法转移（开通/续期/到期回落）。
func TestMembershipLegalTransitions(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	cases := []struct{ from, event, actor, want string }{
		{StateNone, EventActivate, "user", StateActive},    // 新开通
		{StateExpired, EventActivate, "user", StateActive}, // 过期重开
		{StateActive, EventActivate, "user", StateActive},  // 续期
		{StateActive, EventExpire, "system", StateExpired}, // 到期回落
	}
	for _, c := range cases {
		got, err := eng.Apply(context.Background(), Machine, 1, c.from, c.event, c.actor)
		if err != nil || got != c.want {
			t.Fatalf("%s --%s(%s)--> 期望 %s，实际 %s err=%v", c.from, c.event, c.actor, c.want, got, err)
		}
	}
}

// 表驱动：状态机 #12 非法转移必须被拒绝。
func TestMembershipIllegalTransitions(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	cases := []struct{ from, event, actor string }{
		{StateNone, EventExpire, "system"},    // 未开通不可到期
		{StateExpired, EventExpire, "system"}, // 已过期不可再到期
		{StateActive, EventExpire, "user"},    // 到期回落只能由系统驱动
	}
	for _, c := range cases {
		if _, err := eng.Apply(context.Background(), Machine, 1, c.from, c.event, c.actor); err == nil {
			t.Fatalf("非法转移应被拒绝: %s --%s(%s)", c.from, c.event, c.actor)
		}
	}
}
