package domain

import (
	"context"
	"testing"

	"catcube/internal/kernel/statemachine"
)

func TestRealnameMachineValid(t *testing.T) {
	if err := statemachine.Validate(RealnameMachine); err != nil {
		t.Fatalf("状态机定义非法: %v", err)
	}
}

// 表驱动：状态机 #1 合法转移。
func TestRealnameLegalTransitions(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	cases := []struct{ from, event, actor, want string }{
		{StateNone, EventSubmit, "user", StateReviewing},
		{StateFailed, EventRetry, "user", StateReviewing},
		{StateReviewing, EventPass, "system", StateVerified},
		{StateReviewing, EventFail, "system", StateFailed},
	}
	for _, c := range cases {
		got, err := eng.Apply(context.Background(), RealnameMachine, 1, c.from, c.event, c.actor)
		if err != nil || got != c.want {
			t.Fatalf("%s --%s(%s)--> 期望 %s，实际 %s err=%v", c.from, c.event, c.actor, c.want, got, err)
		}
	}
}

// 表驱动：非法转移必须被引擎拒绝。
func TestRealnameIllegalTransitions(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	cases := []struct{ from, event, actor string }{
		{StateVerified, EventSubmit, "user"},  // 终态不可再提交
		{StateVerified, EventFail, "system"},  // 终态无出边
		{StateNone, EventPass, "system"},      // 未进入核验中不可直接通过
		{StateNone, EventFail, "system"},      // 未进入核验中不可直接失败
		{StateReviewing, EventSubmit, "user"}, // 核验中不可重复提交
		{StateReviewing, EventPass, "user"},   // 结论只能由系统（核验接口）驱动
		{StateFailed, EventSubmit, "user"},    // 失败后须走 retry
	}
	for _, c := range cases {
		if _, err := eng.Apply(context.Background(), RealnameMachine, 1, c.from, c.event, c.actor); err == nil {
			t.Fatalf("非法转移应被拒绝: %s --%s(%s)", c.from, c.event, c.actor)
		}
	}
}

func TestStateOf(t *testing.T) {
	cases := []struct {
		status int16
		has    bool
		want   string
	}{
		{0, false, StateNone},
		{0, true, StateReviewing},
		{1, true, StateVerified},
		{2, true, StateFailed},
	}
	for _, c := range cases {
		if got := StateOf(c.status, c.has); got != c.want {
			t.Fatalf("StateOf(%d,%v)=%s want %s", c.status, c.has, got, c.want)
		}
	}
}

func TestValidIDCardAndName(t *testing.T) {
	if !ValidIDCard("11010119900307001X") || !ValidIDCard("110101199003070019") {
		t.Fatal("合法身份证号应通过")
	}
	if ValidIDCard("11010119900307001") || ValidIDCard("11010119900307001Y") || ValidIDCard("") {
		t.Fatal("非法身份证号应拒绝")
	}
	if !ValidName("张三") || ValidName("  ") {
		t.Fatal("姓名校验不符")
	}
}
