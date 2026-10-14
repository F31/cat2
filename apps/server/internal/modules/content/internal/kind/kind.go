// Package kind 四类内容的 ContentKind 插件：字段 schema 与校验、卡片/详情/会员可见字段、
// 审核文本/图片提取、SearchDoc 映射。新增内容类型只需注册一个 Kind + 一张子表。
package kind

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// FieldType 字段类型。
type FieldType string

const (
	TypeText    FieldType = "text"
	TypeInt     FieldType = "int"
	TypeDecimal FieldType = "decimal"
	TypeTime    FieldType = "time"
	TypeCodes   FieldType = "codes"
	TypePhone   FieldType = "phone"
)

// FieldDef 字段定义。
type FieldDef struct {
	Key      string
	Type     FieldType
	Required bool
	MaxLen   int  // 文本按字符数
	Member   bool // 会员可见
}

// Kind 内容类型插件。
type Kind interface {
	Name() string
	Fields() []FieldDef
	CardFields() []string
	DetailFields() []string
	MemberFields() []string
	ReviewText(title string, fields map[string]any) string
	ReviewImages(coverFileID int64, fields map[string]any) []string
}

// Registry ContentKind 注册表。
type Registry struct{ kinds map[string]Kind }

func NewRegistry() *Registry { return &Registry{kinds: map[string]Kind{}} }

func (r *Registry) Register(k Kind) { r.kinds[k.Name()] = k }

func (r *Registry) Get(name string) (Kind, bool) {
	k, ok := r.kinds[name]
	return k, ok
}

// DefaultRegistry 注册四类内容。
func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, k := range []Kind{projectKind{}, productKind{}, meetingKind{}, eventKind{}} {
		r.Register(k)
	}
	return r
}

// Validate 按 Kind 的字段 schema 校验（必填、类型、长度；稳定顺序返回第一个错误）。
func Validate(k Kind, fields map[string]any) error {
	for _, f := range k.Fields() {
		v, ok := fields[f.Key]
		if !ok || isEmpty(v) {
			if f.Required {
				return &FieldError{Field: f.Key, Msg: "必填"}
			}
			continue
		}
		if err := checkType(f, v); err != nil {
			return err
		}
		if f.MaxLen > 0 {
			if s, ok := v.(string); ok && utf8.RuneCountInString(s) > f.MaxLen {
				return &FieldError{Field: f.Key, Msg: fmt.Sprintf("超长（≤%d字）", f.MaxLen)}
			}
		}
	}
	return nil
}

// FieldError 字段校验错误（供前端定位第一个未填必填项）。
type FieldError struct {
	Field string
	Msg   string
}

func (e *FieldError) Error() string { return fmt.Sprintf("content: 字段 %s %s", e.Field, e.Msg) }

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case []string:
		return len(t) == 0
	}
	return false
}

func checkType(f FieldDef, v any) error {
	switch f.Type {
	case TypeText, TypePhone:
		if _, ok := v.(string); !ok {
			return &FieldError{Field: f.Key, Msg: "应为字符串"}
		}
	case TypeInt:
		switch v.(type) {
		case int, int64, float64:
		default:
			return &FieldError{Field: f.Key, Msg: "应为整数"}
		}
	case TypeDecimal:
		switch v.(type) {
		case int, int64, float64:
		default:
			return &FieldError{Field: f.Key, Msg: "应为数字"}
		}
	case TypeTime:
		if _, ok := v.(string); !ok {
			return &FieldError{Field: f.Key, Msg: "应为 RFC3339 时间"}
		}
	case TypeCodes:
		if _, ok := v.([]any); !ok {
			if _, ok := v.([]string); !ok {
				return &FieldError{Field: f.Key, Msg: "应为 code 数组"}
			}
		}
	}
	return nil
}

// textOf 读取文本字段。
func textOf(fields map[string]any, key string) string {
	if v, ok := fields[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// codesOf 读取 code 数组。
func codesOf(fields map[string]any, key string) []string {
	switch t := fields[key].(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
