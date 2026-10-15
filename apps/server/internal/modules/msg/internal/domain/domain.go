// Package domain msg 领域规则：单聊唯一键、消息校验、分页（纯函数）。
package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// 领域错误（映射到错误码 07xxxx）。
var (
	ErrInvalidTarget   = errors.New("msg: 目标引用非法（仅支持 user:<id>）")
	ErrSelfSession     = errors.New("msg: 不能与自己发起会话")
	ErrMemberRequired  = errors.New("msg: 发起单聊需要会员权益")
	ErrSessionNotFound = errors.New("msg: 会话不存在")
	ErrNotParticipant  = errors.New("msg: 非会话成员")
	ErrBadMsgType      = errors.New("msg: 不支持的消息类型")
	ErrEmptyContent    = errors.New("msg: 消息内容不能为空")
	ErrContentTooLong  = errors.New("msg: 消息内容超长")
)

// 校验阈值。
const (
	MaxTextLen  = 1000 // 文本消息最大字数
	DefaultPage = 20
	MaxPage     = 50
)

// DMKey 单聊唯一键：双方 uid 排序后取 (min,max)。
func DMKey(a, b int64) (min, max int64) {
	if a <= b {
		return a, b
	}
	return b, a
}

// ValidMsgType 一期：文本/图片。
func ValidMsgType(t int16) bool { return t == 1 || t == 2 }

// ValidateContent 校验消息内容（图片消息存 file_id，非空即可）。
func ValidateContent(msgType int16, content string) error {
	if !ValidMsgType(msgType) {
		return ErrBadMsgType
	}
	if strings.TrimSpace(content) == "" {
		return ErrEmptyContent
	}
	if msgType == 1 && utf8.RuneCountInString(content) > MaxTextLen {
		return ErrContentTooLong
	}
	return nil
}

// ClampLimit 分页大小归一（1..MaxPage）。
func ClampLimit(n int) int {
	if n <= 0 {
		return DefaultPage
	}
	if n > MaxPage {
		return MaxPage
	}
	return n
}
