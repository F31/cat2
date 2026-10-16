// Package module 定义模块接口、注册表与依赖拓扑装载器。
// 规则：模块只能依赖编号更低的阶（同阶不可）；缺依赖启动即失败。
package module

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/quota"
	"catcube/internal/kernel/settings"
	"catcube/internal/kernel/statemachine"
)

// Rank 依赖阶梯（R0 最低）。
type Rank int

const (
	RankShared Rank = iota // dict、media、entityref
	RankR0                 // identity、trust、ai
	RankR1                 // profile、msg、trade、social
	RankR2                 // verify、content
	RankR3                 // membership、cube
	RankR4                 // circle、channel
	RankR5                 // discovery
	RankR6                 // notice
)

// Module 业务模块。实现放 internal/，对外只有 contract/。
type Module interface {
	Name() string
	Rank() Rank
	DependsOn() []string
	Register(r *Registry) error
}

// Route HTTP 路由注册项。
type Route struct {
	Method  string
	Path    string // /api/{mp|admin}/v1/<module>/<resource>
	Handler http.HandlerFunc
	Admin   bool   // true=后台路由（RBAC 过滤）
	Perm    string // 非空=后台操作级权限点（<module>:<resource>:<action>，p4 §3.2）
}

// AdminSection 后台菜单声明段（p1 §9.9/§11.7；菜单级+操作级权限点细化在 p4）。
type AdminSection struct {
	Key    string // 稳定键，如 identity.users
	Label  string // 菜单名
	Path   string // 前端路由，如 /users
	Module string // 归属模块
}

// JobDef 定时任务（scheduler 执行，PG 咨询锁单活）。
type JobDef struct {
	Name     string
	Interval time.Duration
	Run      func(ctx context.Context) error
}

// Registry 模块注册表：Register 时把路由/事件/任务/策略/状态机/配置/配额挂进来。
type Registry struct {
	ModuleName string

	Routes        []Route
	EventSubs     []eventbus.Subscription
	Jobs          []JobDef
	StateMachines []statemachine.Def
	AdminSections []AdminSection
	Settings      []settings.Schema
	Quotas        []quota.Def
	PolicyRules   []policy.Rule
	AuditPoints   []audit.Point
	CapProviders  []policy.CapabilityProvider
	// Registries 扩展点注册表开口（低阶开口，高阶注册实现）
	Extensions map[string]any
}

// NewRegistry 为模块创建注册表。
func NewRegistry(moduleName string) *Registry {
	return &Registry{ModuleName: moduleName, Extensions: map[string]any{}}
}

// Load 按依赖拓扑排序装载模块：缺依赖或成环直接失败。
// 同时校验依赖阶梯：不得依赖同阶或更高阶模块。
func Load(mods []Module) ([]Module, error) {
	byName := map[string]Module{}
	for _, m := range mods {
		if _, dup := byName[m.Name()]; dup {
			return nil, fmt.Errorf("module: 重复模块 %q", m.Name())
		}
		byName[m.Name()] = m
	}
	var ordered []Module
	visited := map[string]int{} // 0=未访问 1=访问中 2=完成
	var visit func(name string, stack []string) error
	visit = func(name string, stack []string) error {
		switch visited[name] {
		case 1:
			return fmt.Errorf("module: 依赖成环 %v -> %s", stack, name)
		case 2:
			return nil
		}
		m, ok := byName[name]
		if !ok {
			return fmt.Errorf("module: 缺失依赖 %q（被 %v 依赖）", name, stack)
		}
		visited[name] = 1
		for _, dep := range m.DependsOn() {
			d, ok := byName[dep]
			if !ok {
				return fmt.Errorf("module: %q 依赖缺失的 %q", name, dep)
			}
			if d.Rank() >= m.Rank() && d.Rank() != RankShared {
				return fmt.Errorf("module: %q(R%d) 不得依赖同阶或高阶的 %q(R%d)", name, m.Rank(), dep, d.Rank())
			}
			if err := visit(dep, append(stack, name)); err != nil {
				return err
			}
		}
		visited[name] = 2
		ordered = append(ordered, m)
		return nil
	}
	for _, m := range mods {
		if err := visit(m.Name(), nil); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
