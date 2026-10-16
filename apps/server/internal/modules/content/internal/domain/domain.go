// Package domain content 领域规则：状态机 #2、标题校验、会员可见字段过滤（纯函数）。
package domain

import (
	"errors"
	"strings"
	"unicode/utf8"

	"catcube/internal/kernel/statemachine"
)

// 领域错误（映射到错误码 11xxxx）。
var (
	ErrNotFound      = errors.New("content: 内容不存在")
	ErrForbidden     = errors.New("content: 无权操作该内容")
	ErrTitleRequired = errors.New("content: 标题必填")
	ErrTitleTooLong  = errors.New("content: 标题超长（≤50字）")
	ErrNotRealname   = errors.New("content: 发布需先完成实名认证")
	ErrRestricted    = errors.New("content: 账号处于限制期，禁止发布")
	ErrKindUnknown   = errors.New("content: 未知内容类型")
)

// 状态机 #2 状态。
const (
	StateReviewing = "reviewing"  // 审核中
	StatePublished = "published"  // 展示中
	StateTakenDown = "taken_down" // 已下架
)

// 状态机 #2 事件。
const (
	EventPublish  = "publish"
	EventReject   = "reject"
	EventTakeDown = "takedown"
	EventClose    = "close"
)

// Machine 状态机 #2：审核中→展示中→已下架；已下架为终态（不可自行恢复，可申诉）。
var Machine = statemachine.Def{
	Name:     "content.item",
	States:   []string{StateReviewing, StatePublished, StateTakenDown},
	Terminal: []string{StateTakenDown},
	Transitions: []statemachine.Transition{
		{From: StateReviewing, Event: EventPublish, To: StatePublished, Actor: "system"},
		{From: StateReviewing, Event: EventReject, To: StateTakenDown, Actor: "system"},
		{From: StatePublished, Event: EventTakeDown, To: StateTakenDown, Actor: "*"},
		{From: StatePublished, Event: EventClose, To: StateTakenDown, Actor: "*"},
	},
}

// ValidTitle 标题 ≤50 字且非空。
func ValidTitle(s string) error {
	t := strings.TrimSpace(s)
	if t == "" {
		return ErrTitleRequired
	}
	if utf8.RuneCountInString(t) > 50 {
		return ErrTitleTooLong
	}
	return nil
}

// FilterMember 非会员时移除会员可见字段；返回过滤后字段、被隐藏字段与解锁 Reason。
func FilterMember(fields map[string]any, memberFields []string, isMember bool) (map[string]any, []string, string) {
	if isMember {
		return fields, nil, ""
	}
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		out[k] = v
	}
	var hidden []string
	for _, k := range memberFields {
		if _, ok := out[k]; ok {
			delete(out, k)
			hidden = append(hidden, k)
		}
	}
	reason := ""
	if len(hidden) > 0 {
		reason = "开通会员后可查看核心优势与联系方式"
	}
	return out, hidden, reason
}
