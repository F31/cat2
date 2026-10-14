// Package entityref 实体引用（轻量包，无独立表）。
// EntityRef{type,id}：收藏/关注、举报、挂载、关联项目、审核任务、审计、搜索回填统一走本包，
// 杜绝每个模块自己的 targetType 枚举。
package entityref

import (
	"context"
	"fmt"
)

// Ref 实体引用。
type Ref struct {
	Type string `json:"type"` // user/org/content.project/content.product/content.meeting/content.event/circle/cube/cube.project...
	ID   int64  `json:"id,string"`
}

func (r Ref) String() string { return fmt.Sprintf("%s:%d", r.Type, r.ID) }

// Info 引用解析结果（展示信息与可见性）。
type Info struct {
	Title      string `json:"title"`
	Cover      string `json:"cover,omitempty"`
	Status     string `json:"status"`
	OwnerRef   *Ref   `json:"owner_ref,omitempty"`
	Visibility string `json:"visibility"` // platform / space
}

// Resolver 实体解析器：由各模块注册（profile/content/circle/cube）。
type Resolver interface {
	Resolve(ctx context.Context, ref Ref) (Info, error)
}

// Registry 解析器注册表（扩展点 #4）。
type Registry struct {
	resolvers map[string]Resolver
}

func NewRegistry() *Registry { return &Registry{resolvers: map[string]Resolver{}} }

// Register 注册某类型的解析器（注册即生效，删除模块即失效）。
func (r *Registry) Register(refType string, resolver Resolver) {
	r.resolvers[refType] = resolver
}

// Resolve 解析单个引用。
func (r *Registry) Resolve(ctx context.Context, ref Ref) (Info, error) {
	res, ok := r.resolvers[ref.Type]
	if !ok {
		return Info{}, fmt.Errorf("entityref: 未注册的类型 %q", ref.Type)
	}
	return res.Resolve(ctx, ref)
}

// BatchResolve 批量解析。
func (r *Registry) BatchResolve(ctx context.Context, refs []Ref) ([]Info, error) {
	out := make([]Info, len(refs))
	for i, ref := range refs {
		info, err := r.Resolve(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = info
	}
	return out, nil
}
