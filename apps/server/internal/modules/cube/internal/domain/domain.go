// Package domain cube 领域：状态机 #5 猫立方 / #6 成员，档位与校验规则（无 I/O）。
package domain

import (
	"errors"
	"strings"

	"catcube/internal/kernel/statemachine"
)

// 状态常量（与 contract 对齐，避免 domain 依赖 contract）。
const (
	CubeActive    = "active"
	CubeBanned    = "banned"
	CubeDissolved = "dissolved"

	MemberActive  = "active"
	MemberLeft    = "left"
	MemberRemoved = "removed"
)

// 事件。
const (
	EventBan       = "ban"
	EventUnban     = "unban"
	EventDissolve  = "dissolve"
	EventLeave     = "leave"
	EventRemove    = "remove"
	EventRejoin    = "rejoin"
)

// MachineCube 状态机 #5 猫立方：正常→封禁→已解散（封禁可解封）。
var MachineCube = statemachine.Def{
	Name:     "cube.cube",
	States:   []string{CubeActive, CubeBanned, CubeDissolved},
	Terminal: []string{CubeDissolved},
	Transitions: []statemachine.Transition{
		{From: CubeActive, Event: EventBan, To: CubeBanned, Actor: "admin"},
		{From: CubeBanned, Event: EventUnban, To: CubeActive, Actor: "admin"},
		{From: CubeActive, Event: EventDissolve, To: CubeDissolved, Actor: "leader"},
		{From: CubeBanned, Event: EventDissolve, To: CubeDissolved, Actor: "admin"},
	},
}

// MachineMember 状态机 #6 成员：正常→已退出/被移除（被移除不可自行重进）。
var MachineMember = statemachine.Def{
	Name:     "cube.member",
	States:   []string{MemberActive, MemberLeft, MemberRemoved},
	Terminal: []string{MemberRemoved},
	Transitions: []statemachine.Transition{
		{From: MemberActive, Event: EventLeave, To: MemberLeft, Actor: "self"},
		{From: MemberActive, Event: EventRemove, To: MemberRemoved, Actor: "leader"},
		{From: MemberLeft, Event: EventRejoin, To: MemberActive, Actor: "invite"},
	},
}

// 档位定义（默认值，实际以 settings 为准，后台可配）。
type PlanTier struct {
	Level       int16
	Name        string
	CubeLimit   int // 可加入立方数；0 表示不限
	MemberLimit int // 单立方成员上限；0 表示不限
	PriceFen    int64
}

// DefaultTiers 三档默认（基础免费 1/≤10；进阶 3/≤50 ¥999；高阶不限/不限 ¥2999）。
var DefaultTiers = []PlanTier{
	{Level: 1, Name: "基础", CubeLimit: 1, MemberLimit: 10, PriceFen: 0},
	{Level: 2, Name: "进阶", CubeLimit: 3, MemberLimit: 50, PriceFen: 99900},
	{Level: 3, Name: "高阶", CubeLimit: 0, MemberLimit: 0, PriceFen: 299900},
}

// Tier 取档位定义（不存在返回基础档）。
func Tier(level int16) PlanTier {
	for _, t := range DefaultTiers {
		if t.Level == level {
			return t
		}
	}
	return DefaultTiers[0]
}

// ValidTier 档位合法。
func ValidTier(level int16) bool {
	return level == 1 || level == 2 || level == 3
}

var (
	ErrNotRealname   = errors.New("cube: 需先完成实名认证")
	ErrHoneyRequired = errors.New("cube: 须指定 1 名猫蜜（不可为自己）")
	ErrInviteInvalid = errors.New("cube: 邀请码无效或已过期")
	ErrRemoved       = errors.New("cube: 已被移除，须重新受邀加入")
	ErrCubeLimit     = errors.New("cube: 已达可加入立方数上限")
	ErrMemberLimit   = errors.New("cube: 单立方成员数已达上限")
	ErrInvalidInput  = errors.New("cube: 参数校验失败")
	ErrNoPermission  = errors.New("cube: 无权操作该猫立方")
	ErrNotFound      = errors.New("cube: 猫立方不存在")
	ErrBadTier       = errors.New("cube: 订阅档位非法")
)

const nameMaxLen = 64

// ValidateCreate 创建入参校验。
func ValidateCreate(name string, honeyUserID, creatorID int64) error {
	if strings.TrimSpace(name) == "" || len([]rune(name)) > nameMaxLen {
		return ErrInvalidInput
	}
	if honeyUserID == 0 || honeyUserID == creatorID {
		return ErrHoneyRequired
	}
	return nil
}

// CanJoin 判断成员是否可加入（未被该立方移除过）。
func CanJoin(memberStatus int16, exists bool) error {
	if exists && memberStatus == 2 { // 被移除
		return ErrRemoved
	}
	return nil
}
