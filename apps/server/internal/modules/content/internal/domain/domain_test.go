package domain

import (
	"context"
	"testing"

	"catcube/internal/kernel/statemachine"
)

func TestMachineValid(t *testing.T) {
	if err := statemachine.Validate(Machine); err != nil {
		t.Fatalf("状态机 #2 定义非法: %v", err)
	}
}

// 表驱动：状态机 #2 合法转移。
func TestLegalTransitions(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	cases := []struct{ from, event, actor, want string }{
		{StateReviewing, EventPublish, "system", StatePublished},
		{StateReviewing, EventReject, "system", StateTakenDown},
		{StatePublished, EventTakeDown, "admin", StateTakenDown},
		{StatePublished, EventClose, "user", StateTakenDown},
	}
	for _, c := range cases {
		got, err := eng.Apply(context.Background(), Machine, 1, c.from, c.event, c.actor)
		if err != nil || got != c.want {
			t.Fatalf("%s --%s(%s)--> 期望 %s，实际 %s err=%v", c.from, c.event, c.actor, c.want, got, err)
		}
	}
}

// 表驱动：非法转移被拒绝（已下架不可自行恢复）。
func TestIllegalTransitions(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	cases := []struct{ from, event, actor string }{
		{StateTakenDown, EventPublish, "system"}, // 终态无出边
		{StateTakenDown, EventClose, "user"},
		{StateReviewing, EventClose, "user"},     // 审核中不可直接关闭（须先展示）
		{StatePublished, EventPublish, "system"}, // 展示中不可重复发布
		{StatePublished, EventReject, "system"},
		{StateReviewing, EventPublish, "user"}, // 发布结论只能由系统（审核）驱动
	}
	for _, c := range cases {
		if _, err := eng.Apply(context.Background(), Machine, 1, c.from, c.event, c.actor); err == nil {
			t.Fatalf("非法转移应被拒绝: %s --%s(%s)", c.from, c.event, c.actor)
		}
	}
}

func TestValidTitle(t *testing.T) {
	if ValidTitle("") != ErrTitleRequired {
		t.Fatal("空标题应拒绝")
	}
	long := make([]rune, 51)
	for i := range long {
		long[i] = '标'
	}
	if ValidTitle(string(long)) != ErrTitleTooLong {
		t.Fatal("超 50 字应拒绝")
	}
	if err := ValidTitle("一个项目"); err != nil {
		t.Fatalf("合法标题应通过: %v", err)
	}
}

func TestFilterMember(t *testing.T) {
	fields := map[string]any{"amount_wan": 100.0, "core_strength": "优势", "contact_phone": "138"}
	member := []string{"core_strength", "contact_phone"}

	out, hidden, reason := FilterMember(fields, member, true)
	if len(hidden) != 0 || reason != "" || out["core_strength"] != "优势" {
		t.Fatalf("会员应保留全部字段: %v %v %q", out, hidden, reason)
	}

	out, hidden, reason = FilterMember(fields, member, false)
	if _, ok := out["core_strength"]; ok {
		t.Fatal("非会员应隐藏核心优势")
	}
	if _, ok := out["contact_phone"]; ok {
		t.Fatal("非会员应隐藏联系方式")
	}
	if out["amount_wan"] == nil || len(hidden) != 2 || reason == "" {
		t.Fatalf("非会员过滤不符: %v %v %q", out, hidden, reason)
	}
}
