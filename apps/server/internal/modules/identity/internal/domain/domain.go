// Package domain identity 领域规则：锁定策略、会话上限、状态判定（纯函数，无 IO）。
package domain

import (
	"errors"
	"time"
)

// 领域错误（映射到错误码 01xxxx）。
var (
	ErrUserNotFound       = errors.New("identity: 用户不存在")
	ErrPhoneRequired      = errors.New("identity: 缺少手机号")
	ErrAccountDeactivated = errors.New("identity: 账号已注销")
	ErrAccountBanned      = errors.New("identity: 账号被停用")
	ErrLocked             = errors.New("identity: 尝试过于频繁，请稍后再试")
	ErrCodeInvalid        = errors.New("identity: 验证码无效或已过期")
	ErrRefreshInvalid     = errors.New("identity: refresh token 无效")
	ErrWeChatUpstream     = errors.New("identity: 微信接口失败")
)

// StatusError 账号状态不允许登录时的领域错误（封禁 / 注销分别归因）。
func StatusError(s UserStatus) error {
	switch s {
	case StatusDeactivated:
		return ErrAccountDeactivated
	case StatusBanned:
		return ErrAccountBanned
	default:
		return ErrUserNotFound
	}
}

// 登录失败锁定策略：窗口内失败次数达到阈值即锁定。
const (
	FailThreshold = 5
	FailWindow    = 10 * time.Minute
	MaxSessions   = 3 // 同账号活跃会话上限
)

// Locked 给定窗口内失败次数与最近一次失败时间，判断是否处于锁定。
func Locked(failures int, lastFail time.Time, now time.Time) bool {
	if failures < FailThreshold {
		return false
	}
	return now.Sub(lastFail) < FailWindow
}

// SessionsToRevoke 保证新会话可用：现有活跃会话数 + 1 > MaxSessions 时，
// 按创建时间从旧到新返回需要吊销的会话 ID（最旧优先）。
func SessionsToRevoke(existing []SessionBrief, max int) []int64 {
	need := len(existing) + 1 - max
	if need <= 0 {
		return nil
	}
	ids := make([]int64, 0, need)
	// existing 需按 CreatedAt 升序
	for i := 0; i < need && i < len(existing); i++ {
		ids = append(ids, existing[i].ID)
	}
	return ids
}

// SessionBrief 会话摘要（排序输入）。
type SessionBrief struct {
	ID        int64
	CreatedAt time.Time
}

// UserStatus 账号状态。
type UserStatus int16

const (
	StatusActive      UserStatus = 0
	StatusRestricted  UserStatus = 1
	StatusBanned      UserStatus = 2
	StatusDeactivated UserStatus = 3

	RealnameNone     int16 = 0
	RealnameVerified int16 = 1
	RealnameFailed   int16 = 2
)

// CanLogin 账号是否允许登录（限制可登录，封禁/注销不可）。
func CanLogin(s UserStatus) bool {
	return s == StatusActive || s == StatusRestricted
}

// ValidPhone 手机号格式（中国大陆 11 位，1 开头，第二位 3-9）。
func ValidPhone(p string) bool {
	if len(p) != 11 || p[0] != '1' {
		return false
	}
	return p[1] >= '3' && p[1] <= '9'
}
