// Package domain circle 领域：状态机 #10 圈子 / #11 圈内需求，类型与配额规则（无 I/O）。
package domain

import (
	"errors"
	"strings"

	"catcube/internal/kernel/statemachine"
)

// 圈子状态字符串（状态机 #10）。
const (
	CircleActive    = "active"
	CircleEnded     = "ended"
	CircleDissolved = "dissolved"
	CircleBanned    = "banned"
	CircleArchived  = "archived"

	EventEnd           = "end"
	EventConvert       = "convert" // 转常设（已结束→进行中）
	EventDissolve      = "dissolve"
	EventBan           = "ban"
	EventArchive       = "archive"
	EventUnbanActive   = "unban_active"
	EventUnbanEnded    = "unban_ended"
	EventRestoreActive = "restore_active"
	EventRestoreEnded  = "restore_ended"
)

// 需求状态字符串（状态机 #11）。
const (
	DemandValid   = "valid"
	DemandExpired = "expired"
	EventExpire   = "expire"
)

// MachineCircle 状态机 #10：进行中→已结束→已解散；平台封禁/归档可恢复。
var MachineCircle = statemachine.Def{
	Name:     "circle.circle",
	States:   []string{CircleActive, CircleEnded, CircleDissolved, CircleBanned, CircleArchived},
	Terminal: []string{CircleDissolved},
	Transitions: []statemachine.Transition{
		{From: CircleActive, Event: EventEnd, To: CircleEnded, Actor: "system"},
		{From: CircleEnded, Event: EventConvert, To: CircleActive, Actor: "owner"},
		{From: CircleActive, Event: EventDissolve, To: CircleDissolved, Actor: "owner"},
		{From: CircleEnded, Event: EventDissolve, To: CircleDissolved, Actor: "owner"},
		{From: CircleBanned, Event: EventDissolve, To: CircleDissolved, Actor: "admin"},
		{From: CircleArchived, Event: EventDissolve, To: CircleDissolved, Actor: "admin"},
		{From: CircleActive, Event: EventBan, To: CircleBanned, Actor: "admin"},
		{From: CircleEnded, Event: EventBan, To: CircleBanned, Actor: "admin"},
		{From: CircleActive, Event: EventArchive, To: CircleArchived, Actor: "admin"},
		{From: CircleEnded, Event: EventArchive, To: CircleArchived, Actor: "admin"},
		{From: CircleBanned, Event: EventUnbanActive, To: CircleActive, Actor: "admin"},
		{From: CircleBanned, Event: EventUnbanEnded, To: CircleEnded, Actor: "admin"},
		{From: CircleArchived, Event: EventRestoreActive, To: CircleActive, Actor: "admin"},
		{From: CircleArchived, Event: EventRestoreEnded, To: CircleEnded, Actor: "admin"},
	},
}

// MachineDemand 状态机 #11：有效→已过期（沉底可续期 30 天）。
var MachineDemand = statemachine.Def{
	Name:     "circle.demand",
	States:   []string{DemandValid, DemandExpired},
	Terminal: []string{DemandExpired},
	Transitions: []statemachine.Transition{
		{From: DemandValid, Event: EventExpire, To: DemandExpired, Actor: "system"},
	},
}

var (
	ErrNotRealname   = errors.New("circle: 需先完成实名认证")
	ErrCircleLimit   = errors.New("circle: 建圈已达上限（每人每日 1 个 / 同时最多 3 个）")
	ErrDuration      = errors.New("circle: 临时圈时长非法（7/30/90/180）")
	ErrNotFound      = errors.New("circle: 圈子不存在或不可见")
	ErrNoPermission  = errors.New("circle: 无权操作该圈子")
	ErrInviteInvalid = errors.New("circle: 邀请链接无效或已失效")
	ErrInvalidInput  = errors.New("circle: 需求/动态字段校验失败")
	ErrPostQuota     = errors.New("circle: 圈内发布每日额度已满（跨圈合计 3 条）")
	ErrAnnounceQuota = errors.New("circle: 公告每日额度已满（每圈 3 条）")
	ErrMountLimit    = errors.New("circle: 挂载数已达单圈上限")
	ErrEnded         = errors.New("circle: 圈子已结束（只读）")
	ErrInvalidState  = errors.New("circle: 当前状态不允许该操作")
)

const (
	validDurations = "7,30,90,180"
	nameMax        = 64
	titleMax       = 30
	contentMax     = 500
)

// ValidDuration 临时圈时长合法（7/30/90/180）。
func ValidDuration(d int) bool {
	return d == 7 || d == 30 || d == 90 || d == 180
}

// ValidateCreate 建圈校验。
func ValidateCreate(name string, typ int16, durationDays int) error {
	if strings.TrimSpace(name) == "" || len([]rune(name)) > nameMax {
		return ErrInvalidInput
	}
	if typ == 2 && !ValidDuration(durationDays) {
		return ErrDuration
	}
	return nil
}

// ValidateDemand 需求字段校验（标题 ≤30 字、内容 ≤500 字）。
func ValidateDemand(title, content string) error {
	if strings.TrimSpace(title) == "" || len([]rune(title)) > titleMax {
		return ErrInvalidInput
	}
	if strings.TrimSpace(content) == "" || len([]rune(content)) > contentMax {
		return ErrInvalidInput
	}
	return nil
}

var _ = validDurations
