// Package errs 提供统一错误类型与错误码映射。错误码 MMNNNN：前两位模块号，后四位序号。
package errs

import "fmt"

// Kind 错误大类，决定 HTTP 状态码与前端提示方式。
type Kind int

const (
	KindBadRequest   Kind = iota // 400 参数/校验错误
	KindUnauthorized             // 401 未登录或令牌失效
	KindForbidden                // 403 权限拒绝（带 Reason）
	KindNotFound                 // 404
	KindConflict                 // 409 状态冲突（非法状态转移、幂等冲突）
	KindRateLimited              // 429 限流/超限（quota）
	KindInternal                 // 500
)

// Error 业务错误。Code 为 MMNNNN 模块错误码，Reason 驱动前端 PolicyGate 提示。
type Error struct {
	Kind   Kind
	Code   string
	Reason string
	Msg    string
}

func (e *Error) Error() string { return fmt.Sprintf("%s %s: %s", e.Code, e.Reason, e.Msg) }

func New(kind Kind, code, reason, msg string) *Error {
	return &Error{Kind: kind, Code: code, Reason: reason, Msg: msg}
}

func BadRequest(code, msg string) *Error  { return New(KindBadRequest, code, "", msg) }
func Forbidden(code, reason string) *Error { return New(KindForbidden, code, reason, "权限拒绝") }
func NotFound(code, msg string) *Error     { return New(KindNotFound, code, "", msg) }
func Conflict(code, msg string) *Error     { return New(KindConflict, code, "", msg) }
func RateLimited(code, reason string) *Error {
	return New(KindRateLimited, code, reason, "超出限额")
}
func Internal(msg string) *Error { return New(KindInternal, "000000", "", msg) }
