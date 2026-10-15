package domain

import (
	"context"
	"testing"

	"catcube/internal/kernel/statemachine"
)

// 状态机 #10 圈子：#11 需求。
func TestMachineCircle(t *testing.T) {
	eng := statemachine.NewEngine(nil)
	ctx := context.Background()
	if _, err := eng.Apply(ctx, MachineCircle, 1, CircleActive, EventEnd, "system"); err != nil {
		t.Fatalf("到期应允许: %v", err)
	}
	if _, err := eng.Apply(ctx, MachineCircle, 1, CircleEnded, EventConvert, "owner"); err != nil {
		t.Fatalf("转常设应允许: %v", err)
	}
	if _, err := eng.Apply(ctx, MachineCircle, 1, CircleActive, EventDissolve, "owner"); err != nil {
		t.Fatalf("解散应允许: %v", err)
	}
	if _, err := eng.Apply(ctx, MachineCircle, 1, CircleDissolved, EventDissolve, "owner"); err == nil {
		t.Fatal("已解散为终态")
	}
	if _, err := eng.Apply(ctx, MachineDemand, 1, DemandValid, EventExpire, "system"); err != nil {
		t.Fatalf("需求到期应允许: %v", err)
	}
	if _, err := eng.Apply(ctx, MachineDemand, 1, DemandExpired, EventExpire, "system"); err == nil {
		t.Fatal("已过期不可再过期")
	}
}

func TestValidateCreateAndDemand(t *testing.T) {
	if err := ValidateCreate("圈子", 2, 30); err != nil {
		t.Fatalf("合法临时圈: %v", err)
	}
	if err := ValidateCreate("圈子", 2, 15); err == nil {
		t.Fatal("非法时长应拒绝")
	}
	if err := ValidateCreate("圈子", 1, 0); err != nil {
		t.Fatalf("常设圈不校验时长: %v", err)
	}
	if err := ValidateDemand("标题", "内容"); err != nil {
		t.Fatalf("合法需求: %v", err)
	}
	if err := ValidateDemand("", "内容"); err == nil {
		t.Fatal("空标题应拒绝")
	}
}
