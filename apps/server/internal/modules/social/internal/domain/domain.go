// Package domain social 领域规则：EntityRef 解析与收藏分类映射（纯函数）。
package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// 领域错误（映射到错误码 09xxxx）。
var (
	ErrInvalidTarget = errors.New("social: 目标引用非法")
	ErrKindRequired  = errors.New("social: 不支持的收藏分类")
)

// Ref EntityRef 投影。
type Ref struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

// ParseRef 解析 "type:id"。
func ParseRef(s string) (Ref, error) {
	i := strings.LastIndex(s, ":")
	if i <= 0 {
		return Ref{}, fmt.Errorf("%w: %q", ErrInvalidTarget, s)
	}
	id, err := strconv.ParseInt(s[i+1:], 10, 64)
	if err != nil || id <= 0 {
		return Ref{}, fmt.Errorf("%w: %q", ErrInvalidTarget, s)
	}
	return Ref{Type: s[:i], ID: id}, nil
}

func (r Ref) String() string { return fmt.Sprintf("%s:%d", r.Type, r.ID) }

// 收藏六类分类。
const (
	KindPerson  = "person"
	KindCompany = "company"
	KindProject = "project"
	KindProduct = "product"
	KindMeeting = "meeting"
	KindEvent   = "event"
)

// KindOf 目标类型 → 收藏分类。
func KindOf(refType string) (string, bool) {
	switch refType {
	case "user":
		return KindPerson, true
	case "org":
		return KindCompany, true
	case "content.project":
		return KindProject, true
	case "content.product":
		return KindProduct, true
	case "content.meeting":
		return KindMeeting, true
	case "content.event":
		return KindEvent, true
	}
	return "", false
}

// ValidKind 是否合法收藏分类（用于 GET 过滤）。
func ValidKind(kind string) bool {
	for _, k := range []string{KindPerson, KindCompany, KindProject, KindProduct, KindMeeting, KindEvent} {
		if k == kind {
			return true
		}
	}
	return kind == ""
}
