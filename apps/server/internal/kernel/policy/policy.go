// Package policy 授权引擎：能力 + 资源内角色 + 状态门。
// 内核只放引擎与 Actor 装配；规则分散在各模块。内核不 import 任何业务模块。
package policy

import (
	"context"
	"sync"
)

// Decision 授权结论。Reason 同时驱动前端提示与 AI 入口灰显。
type Decision struct {
	Allow  bool
	Reason string
}

// Actor 请求主体：由 CapabilityProvider 惰性装配（请求内缓存）。
type Actor struct {
	UserID int64
	Space  int16
	caps   map[string]bool
	mu     sync.RWMutex
}

// CapabilityProvider 能力提供者（identity「已实名」、membership「会员生效中」、
// verify「已认证/组织管理员」、cube「立方内角色」、trust「是否被限制发布」）。
type CapabilityProvider interface {
	Name() string
	// Has 计算某能力，如 Has(ctx, 123, "realname")
	Has(ctx context.Context, userID int64, capability string) (bool, error)
}

// Rule 策略规则（PRD 权限矩阵每格对应一条）。各模块在自己 domain/policy.go 定义。
type Rule struct {
	Module   string
	Action   string // 如 content.publish
	Requires []string
	Reason   string
}

// Engine 授权引擎。
type Engine struct {
	providers  []CapabilityProvider
	rules      []Rule
	dualSource func(ctx context.Context) (string, bool) // 双人复核清单来源（p4 §3.12）
}

func NewEngine() *Engine { return &Engine{} }

func (e *Engine) RegisterProviders(ps []CapabilityProvider) { e.providers = append(e.providers, ps...) }
func (e *Engine) RegisterRules(rs []Rule)                   { e.rules = append(e.rules, rs...) }

// NewActor 创建请求主体。
func (e *Engine) NewActor(userID int64, space int16) *Actor {
	return &Actor{UserID: userID, Space: space, caps: map[string]bool{}}
}

// Has 惰性装配能力（请求内缓存）。
func (e *Engine) Has(ctx context.Context, a *Actor, capability string) (bool, error) {
	a.mu.RLock()
	v, ok := a.caps[capability]
	a.mu.RUnlock()
	if ok {
		return v, nil
	}
	for _, p := range e.providers {
		has, err := p.Has(ctx, a.UserID, capability)
		if err != nil {
			return false, err
		}
		if has {
			a.mu.Lock()
			a.caps[capability] = true
			a.mu.Unlock()
			return true, nil
		}
	}
	a.mu.Lock()
	a.caps[capability] = false
	a.mu.Unlock()
	return false, nil
}

// Check 按模块规则判定。
func (e *Engine) Check(ctx context.Context, a *Actor, module, action string) (Decision, error) {
	for _, r := range e.rules {
		if r.Module != module || r.Action != action {
			continue
		}
		for _, cap := range r.Requires {
			has, err := e.Has(ctx, a, cap)
			if err != nil {
				return Decision{}, err
			}
			if !has {
				return Decision{Allow: false, Reason: r.Reason}, nil
			}
		}
		return Decision{Allow: true}, nil
	}
	return Decision{Allow: true}, nil // 无规则默认放行（矩阵格应显式登记）
}
