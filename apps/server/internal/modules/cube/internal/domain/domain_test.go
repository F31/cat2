package domain

import (
	"context"
	"testing"

	"catcube/internal/kernel/statemachine"
)

// 状态机 #5 猫立方：合法/非法转移。
func TestMachineCube(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	ctx := context.Background()
	valid := []struct {
		from, event, actor, to string
	}{
		{CubeActive, EventBan, "admin", CubeBanned},
		{CubeBanned, EventUnban, "admin", CubeActive},
		{CubeActive, EventDissolve, "leader", CubeDissolved},
		{CubeBanned, EventDissolve, "admin", CubeDissolved},
	}
	for _, c := range valid {
		if _, err := eng.Apply(ctx, MachineCube, 1, c.from, c.event, c.actor); err != nil {
			t.Fatalf("应允许 %s--%s-->%s: %v", c.from, c.event, c.to, err)
		}
	}
	invalid := []struct{ from, event, actor string }{
		{CubeDissolved, EventDissolve, "leader"}, // 终态
		{CubeDissolved, EventUnban, "admin"},
		{CubeActive, EventDissolve, "admin"}, // 非猫王
	}
	for _, c := range invalid {
		if _, err := eng.Apply(ctx, MachineCube, 1, c.from, c.event, c.actor); err == nil {
			t.Fatalf("应拒绝 %s--%s--> (%s)", c.from, c.event, c.actor)
		}
	}
}

// 状态机 #6 成员：正常→退出/被移除；被移除为终态（不可自行重进）。
func TestMachineMember(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	ctx := context.Background()
	if _, err := eng.Apply(ctx, MachineMember, 1, MemberActive, EventLeave, "self"); err != nil {
		t.Fatalf("退出应允许: %v", err)
	}
	if _, err := eng.Apply(ctx, MachineMember, 1, MemberActive, EventRemove, "leader"); err != nil {
		t.Fatalf("移除应允许: %v", err)
	}
	if _, err := eng.Apply(ctx, MachineMember, 1, MemberRemoved, EventRejoin, "invite"); err == nil {
		t.Fatal("被移除为终态，不可重进")
	}
}

func TestValidateCreate(t *testing.T) {
	if err := ValidateCreate("团队", 2, 1); err != nil {
		t.Fatalf("合法创建参数: %v", err)
	}
	if err := ValidateCreate("", 2, 1); err == nil {
		t.Fatal("空名称应拒绝")
	}
	if err := ValidateCreate("团队", 1, 1); err == nil {
		t.Fatal("猫蜜不可为自己")
	}
	if err := ValidateCreate("团队", 0, 1); err == nil {
		t.Fatal("猫蜜必配")
	}
}

func TestCanJoinRemoved(t *testing.T) {
	if err := CanJoin(2, true); err == nil {
		t.Fatal("被移除者不可自行重进")
	}
	if err := CanJoin(0, true); err != nil {
		t.Fatalf("生效成员应可（幂等）: %v", err)
	}
}
