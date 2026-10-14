// Package domain membership 领域规则：会员状态机 #12、报价与退款计算（纯函数）。
package domain

import (
	"errors"
	"time"

	"catcube/internal/kernel/statemachine"
)

// 领域错误（映射到错误码 12xxxx）。
var (
	ErrLevelUnknown   = errors.New("membership: 不支持的会员等级")
	ErrYearsInvalid   = errors.New("membership: 年限非法")
	ErrNotEnterprise  = errors.New("membership: 仅企业版主账号可管理子账号")
	ErrEnterpriseCert = errors.New("membership: 仅已认证企业可开通企业版")
	ErrSubLimit       = errors.New("membership: 子账号已达上限（3）")
	ErrSubNotRealname = errors.New("membership: 子账号须已注册且已实名")
	ErrSubExists      = errors.New("membership: 子账号已存在")
	ErrNotActive      = errors.New("membership: 当前无生效会员")
)

// 状态机 #12 状态。
const (
	StateNone    = "none"    // 未开通
	StateActive  = "active"  // 生效中
	StateExpired = "expired" // 已过期
)

// 状态机 #12 事件。
const (
	EventActivate = "activate"
	EventExpire   = "expire"
)

// Machine 会员状态机 #12：未开通→生效中→已过期（续费/重开可回到生效中）。
var Machine = statemachine.Def{
	Name:   "membership.member",
	States: []string{StateNone, StateActive, StateExpired},
	Transitions: []statemachine.Transition{
		{From: StateNone, Event: EventActivate, To: StateActive, Actor: "*"},
		{From: StateExpired, Event: EventActivate, To: StateActive, Actor: "*"},
		{From: StateActive, Event: EventActivate, To: StateActive, Actor: "*"}, // 续期
		{From: StateActive, Event: EventExpire, To: StateExpired, Actor: "system"},
	},
}

// ValidLevel 一期上架等级（SVIP 预留不上架）。
func ValidLevel(level int16) bool {
	return level == 1 || level == 3
}

// ValidYears 年限 1/3/5 或自定义（>0）。
func ValidYears(years int) bool { return years >= 1 && years <= 10 }

// DiscountCoeff 折扣系数（万分比）：3 年、5 年有折扣，其余无折扣。
func DiscountCoeff(years, c3Permille, c5Permille int) int {
	switch years {
	case 3:
		return c3Permille
	case 5:
		return c5Permille
	default:
		return 10000
	}
}

// Quote 计算原价与折后价（coefficient 为万分比）。
func Quote(unitFen int64, years, coefficient int) (originalFen, totalFen int64) {
	originalFen = unitFen * int64(years)
	totalFen = originalFen * int64(coefficient) / 10000
	return originalFen, totalFen
}

// RefundAmount 退款金额=剩余完整年数×年费×折扣系数（取订单快照）。
// 剩余完整年数=floor((剩余时长+1天)/365天)：1 天容差吸收履约瞬时误差，
// 使刚开通即退计满整年，同时不满整年的余额不计。
func RefundAmount(unitFen int64, years, coefficient int, expiresAt, now time.Time) int64 {
	if years <= 0 || !expiresAt.After(now) {
		return 0
	}
	const year = 365 * 24 * time.Hour
	remainingYears := int((expiresAt.Sub(now) + 24*time.Hour) / year)
	if remainingYears > years {
		remainingYears = years
	}
	return unitFen * int64(remainingYears) * int64(coefficient) / 10000
}

// StateOf 由会员状态推导状态机状态（has=false 表示无记录=未开通）。
func StateOf(status int16, has bool) string {
	if !has {
		return StateNone
	}
	if status == 2 {
		return StateExpired
	}
	return StateActive
}
