// Package validate 提交级校验辅助（PRD 8.9）。
package validate

import "unicode/utf8"

// Field 单项校验结果。
type Field struct {
	Name    string
	Message string
}

// Errors 收集校验错误；First 返回第一个未通过项（提交级校验定位到第一项）。
type Errors []Field

func (e *Errors) Check(ok bool, name, msg string) {
	if !ok {
		*e = append(*e, Field{Name: name, Message: msg})
	}
}

// Required 必填（非空白）。
func (e *Errors) Required(name, value string) {
	e.Check(utf8.RuneCountInString(value) > 0, name, name+"为必填项")
}

// MaxRunes 字数上限（按字符数计）。
func (e *Errors) MaxRunes(name, value string, max int) {
	e.Check(utf8.RuneCountInString(value) <= max, name, name+"超出字数上限")
}

// OK 是否全部通过。
func (e Errors) OK() bool { return len(e) == 0 }

// First 第一个错误。
func (e Errors) First() (Field, bool) {
	if len(e) == 0 {
		return Field{}, false
	}
	return e[0], true
}
