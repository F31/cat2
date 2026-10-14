// Package domain verify 领域规则：实名状态机、证件校验、状态推导（纯函数，无 IO）。
package domain

import (
	"errors"
	"regexp"
	"strings"

	"catcube/internal/kernel/statemachine"
)

// 领域错误（映射到错误码 10xxxx）。
var (
	ErrAgreementRequired   = errors.New("verify: 请先勾选实名认证服务协议")
	ErrInvalidIDCard       = errors.New("verify: 身份证号格式不正确")
	ErrNameRequired        = errors.New("verify: 真实姓名必填")
	ErrNotMatched          = errors.New("verify: 实名核验未通过，请核对姓名与身份证号")
	ErrUnsupportedCertType = errors.New("verify: 暂不支持的认证类型")
	ErrAlreadyVerified     = errors.New("verify: 已完成实名认证")
	ErrUpstream            = errors.New("verify: 核验服务异常")
)

// 实名状态机 #1 的状态与事件。
const (
	StateNone      = "none"      // 未实名
	StateReviewing = "reviewing" // 核验中
	StateVerified  = "verified"  // 已实名
	StateFailed    = "failed"    // 核验失败

	EventSubmit = "submit"
	EventRetry  = "retry"
	EventPass   = "pass"
	EventFail   = "fail"
)

// RealnameMachine 状态机 #1：未实名→核验中→已实名/核验失败（失败可重提）。
var RealnameMachine = statemachine.Def{
	Name:     "verify.realname",
	States:   []string{StateNone, StateReviewing, StateVerified, StateFailed},
	Terminal: []string{StateVerified},
	Transitions: []statemachine.Transition{
		{From: StateNone, Event: EventSubmit, To: StateReviewing, Actor: "*"},
		{From: StateFailed, Event: EventRetry, To: StateReviewing, Actor: "*"},
		{From: StateReviewing, Event: EventPass, To: StateVerified, Actor: "system"},
		{From: StateReviewing, Event: EventFail, To: StateFailed, Actor: "system"},
	},
}

var idCardRe = regexp.MustCompile(`^\d{17}[\dXx]$`)

// ValidIDCard 身份证号（18 位，末位数字或 X）。
func ValidIDCard(s string) bool { return idCardRe.MatchString(strings.TrimSpace(s)) }

// ValidName 真实姓名（非空且 ≤64 字符）。
func ValidName(s string) bool {
	t := strings.TrimSpace(s)
	return t != "" && len([]rune(t)) <= 64
}

// StateOf 由最近一条实名认证记录推导当前状态（无记录=未实名）。
func StateOf(status int16, has bool) string {
	if !has {
		return StateNone
	}
	switch status {
	case 1:
		return StateVerified
	case 2:
		return StateFailed
	default:
		return StateReviewing
	}
}

// RejectReason 实名核验失败原因（可重提）。
const RejectReason = "实名信息与身份证号不一致"
