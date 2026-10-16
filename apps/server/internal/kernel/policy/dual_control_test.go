package policy

import (
	"context"
	"errors"
	"testing"
)

func TestDualControlOps(t *testing.T) {
	ctx := context.Background()
	e := NewEngine()

	// 默认清单
	if !e.DualControlEnabled(ctx, "trade:refund") || e.DualControlEnabled(ctx, "unknown:op") {
		t.Fatalf("默认清单不符: %v", e.DualControlOps(ctx))
	}

	// 覆盖
	e.SetDualControlSource(func(context.Context) (string, bool) { return "a:op, b:op", true })
	if e.DualControlEnabled(ctx, "trade:refund") || !e.DualControlEnabled(ctx, "a:op") || !e.DualControlEnabled(ctx, "b:op") {
		t.Fatalf("覆盖清单未生效: %v", e.DualControlOps(ctx))
	}

	// 关闭
	e.SetDualControlSource(func(context.Context) (string, bool) { return "off", true })
	if len(e.DualControlOps(ctx)) != 0 || e.DualControlEnabled(ctx, "a:op") {
		t.Fatal("off 应关闭全部")
	}
}

func TestCheckMakerChecker(t *testing.T) {
	if err := CheckMakerChecker("trade:refund", 9, 0, 0); err == nil {
		t.Fatal("未复核应拒绝执行")
	}
	if err := CheckMakerChecker("trade:refund", 9, 9, 0); err == nil {
		t.Fatal("复核人不得执行")
	}
	if err := CheckMakerChecker("trade:refund", 9, 8, 8); err == nil {
		t.Fatal("申请人不得自审")
	}
	if err := CheckMakerChecker("trade:refund", 9, 8, 7); err != nil {
		t.Fatalf("复核人≠执行人且≠申请人应放行: %v", err)
	}
}

func TestEnforceDualControl(t *testing.T) {
	ctx := context.Background()
	e := NewEngine() // 默认清单含 trade:refund

	// 未启用的操作直接放行
	if err := e.EnforceDualControl(ctx, "other:op", 1, 1, 1); err != nil {
		t.Fatalf("未启用操作应放行: %v", err)
	}
	// 启用：复核≠执行
	err := e.EnforceDualControl(ctx, "trade:refund", 5, 5, 0)
	var de *DualControlError
	if !errors.As(err, &de) {
		t.Fatalf("应返回 DualControlError: %v", err)
	}
	// 合规放行
	if err := e.EnforceDualControl(ctx, "trade:refund", 6, 5, 4); err != nil {
		t.Fatalf("合规应放行: %v", err)
	}
}
