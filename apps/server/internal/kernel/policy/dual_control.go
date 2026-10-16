// 双人复核（maker-checker）：复核 ≠ 执行（默认同时要求 复核 ≠ 申请）。
// 通用机制，供结算/退款/会员调整/改价/敏感导出等受控操作复用（p4 §3.5/§3.12）。
// 清单可配：`policy.dual_control`（逗号分隔操作键；`off` 关闭）；默认见 DefaultDualControlOps。
package policy

import (
	"context"
	"fmt"
	"strings"
)

// DefaultDualControlOps 默认强制双人复核的操作键（资金/敏感类，建议保持强制）。
var DefaultDualControlOps = []string{
	"trade:refund",            // 订单退款
	"channel:settlement",      // 渠道结算打款
	"membership:adjust",       // 会员人工调整
	"channel:commission_rule", // 佣金规则变更
	"ops:export",              // 敏感导出
}

// DualControlError 双人复核校验失败。
type DualControlError struct{ Op, Reason string }

func (e *DualControlError) Error() string {
	return fmt.Sprintf("双人复核[%s]：%s", e.Op, e.Reason)
}

// SetDualControlSource 注入清单来源（装配根从 settings 读取；缺省用默认清单）。
func (e *Engine) SetDualControlSource(fn func(ctx context.Context) (string, bool)) {
	e.dualSource = fn
}

// DualControlOps 当前生效的操作清单：settings 覆盖默认；`off` 关闭（空且已显式关闭）。
func (e *Engine) DualControlOps(ctx context.Context) []string {
	if e.dualSource != nil {
		if raw, ok := e.dualSource(ctx); ok {
			raw = strings.TrimSpace(raw)
			if strings.EqualFold(raw, "off") {
				return nil
			}
			if raw != "" {
				return splitOps(raw)
			}
		}
	}
	return DefaultDualControlOps
}

// DualControlEnabled 指定操作是否强制双人复核。
func (e *Engine) DualControlEnabled(ctx context.Context, op string) bool {
	for _, o := range e.DualControlOps(ctx) {
		if o == op {
			return true
		}
	}
	return false
}

// EnforceDualControl 执行环节校验：启用时要求 已复核 且 复核≠执行 且 复核≠申请。
// 未启用该操作则直接放行。
func (e *Engine) EnforceDualControl(ctx context.Context, op string, actorID, reviewedBy, makerID int64) error {
	if !e.DualControlEnabled(ctx, op) {
		return nil
	}
	return CheckMakerChecker(op, actorID, reviewedBy, makerID)
}

// CheckMakerChecker 纯校验（便于单测与模块直接调用）。
//   - reviewedBy==0：尚未复核，不得执行；
//   - actorID==reviewedBy：复核人不得执行（复核≠执行，资金红线）；
//   - makerID!=0 且 reviewedBy==makerID：申请人不得自审（复核≠申请）。
func CheckMakerChecker(op string, actorID, reviewedBy, makerID int64) error {
	if reviewedBy == 0 {
		return &DualControlError{Op: op, Reason: "尚未复核，不得执行"}
	}
	if actorID == reviewedBy {
		return &DualControlError{Op: op, Reason: "复核人不得执行（复核≠执行）"}
	}
	if makerID != 0 && reviewedBy == makerID {
		return &DualControlError{Op: op, Reason: "申请人不得自审（复核≠申请）"}
	}
	return nil
}

func splitOps(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
