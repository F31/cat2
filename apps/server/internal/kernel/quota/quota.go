// Package quota 限额引擎：模块注册 QuotaDef，数值来自配置，计数落库。
// 自然日窗口按 Asia/Shanghai；超限返回 Reason。
package quota

import (
	"context"
	"fmt"
	"time"

	"catcube/pkg/clock"
)

// Window 限额窗口。
type Window string

const (
	WindowDay   Window = "day"   // 自然日（Asia/Shanghai）
	WindowTotal Window = "total" // 累计
)

// Scope 限额维度。
type Scope string

const (
	ScopeUser   Scope = "user"
	ScopeCircle Scope = "circle"
	ScopeCube   Scope = "cube"
)

// Def 限额定义（PRD 11.5 每项数字规则 = 一条 QuotaDef）。数值来自 settings。
type Def struct {
	Key       string // 如 circle.post.daily
	Window    Window
	Scope     Scope
	LimitKey  string // settings 键，取限额数值
	Reason    string // 超限 Reason，驱动前端提示
}

// Counter 计数存储（sys_quota_counter）。
type Counter interface {
	// Incr 原子自增并返回新值。
	Incr(ctx context.Context, key string, scope Scope, scopeID int64, day time.Time) (int64, error)
	// Get 当前计数。
	Get(ctx context.Context, key string, scope Scope, scopeID int64, day time.Time) (int64, error)
}

// LimitSource 从 settings 读取限额数值。
type LimitSource func(ctx context.Context, key string) (int64, error)

// ExceededError 超限。
type ExceededError struct {
	Key    string
	Reason string
	Limit  int64
}

func (e *ExceededError) Error() string {
	return fmt.Sprintf("quota %s 超限（%d）: %s", e.Key, e.Limit, e.Reason)
}

// Engine 限额引擎。
type Engine struct {
	defs    map[string]Def
	counter Counter
	limits  LimitSource
}

func NewEngine(counter Counter, limits LimitSource) *Engine {
	return &Engine{defs: map[string]Def{}, counter: counter, limits: limits}
}

// Register 注册限额定义。
func (e *Engine) Register(defs []Def) {
	for _, d := range defs {
		e.defs[d.Key] = d
	}
}

// Check 校验是否超限（不自增）。超限返回 *ExceededError。
func (e *Engine) Check(ctx context.Context, key string, scopeID int64) error {
	def, ok := e.defs[key]
	if !ok {
		return fmt.Errorf("quota: 未注册的限额 %q", key)
	}
	limit, err := e.limits(ctx, def.LimitKey)
	if err != nil {
		return err
	}
	if limit < 0 { // 负数=不限
		return nil
	}
	cur, err := e.counter.Get(ctx, def.Key, def.Scope, scopeID, windowStart(def))
	if err != nil {
		return err
	}
	if cur >= limit {
		return &ExceededError{Key: key, Reason: def.Reason, Limit: limit}
	}
	return nil
}

// Consume 校验并自增（先校验再自增在存储层应原子化；Incr 后复核超限则回退由调用方事务保证）。
func (e *Engine) Consume(ctx context.Context, key string, scopeID int64) error {
	def, ok := e.defs[key]
	if !ok {
		return fmt.Errorf("quota: 未注册的限额 %q", key)
	}
	limit, err := e.limits(ctx, def.LimitKey)
	if err != nil {
		return err
	}
	if limit < 0 {
		return nil
	}
	n, err := e.counter.Incr(ctx, def.Key, def.Scope, scopeID, windowStart(def))
	if err != nil {
		return err
	}
	if n > limit {
		return &ExceededError{Key: key, Reason: def.Reason, Limit: limit}
	}
	return nil
}

func windowStart(def Def) time.Time {
	if def.Window == WindowDay {
		return clock.Today()
	}
	return time.Time{} // total 窗口不分日
}
