// Package scopedrepo 数据范围：所有仓储强制注入 Scope，拿不到裸查询；
// 软状态（已关闭/已删除）默认过滤。
package scopedrepo

import "fmt"

// ScopeLevel 数据范围层级。
type ScopeLevel string

const (
	Platform ScopeLevel = "platform"
	Space    ScopeLevel = "space"
	Org      ScopeLevel = "org"
	Cube     ScopeLevel = "cube"
	Circle   ScopeLevel = "circle"
	Self     ScopeLevel = "self"
)

// Scope 数据范围上下文（space 一期恒为主站 1）。
type Scope struct {
	Level   ScopeLevel
	SpaceID int16
	OwnerID int64 // org/cube/circle/self 维度的归属 ID
	ActorID int64
}

// Main 主站范围（一期默认）。
func Main(actorID int64) Scope {
	return Scope{Level: Platform, SpaceID: 1, ActorID: actorID}
}

// SelfScope 仅本人数据。
func SelfScope(actorID int64) Scope {
	return Scope{Level: Self, SpaceID: 1, OwnerID: actorID, ActorID: actorID}
}

// Filter SQL 片段：软状态过滤 + 范围约束。返回 WHERE 片段与参数。
// includeClosed 仅后台查询显式声明时为 true。
func Filter(s Scope, includeClosed bool) (string, []any) {
	where := "1=1"
	var args []any
	if !includeClosed {
		where += " AND closed_at IS NULL"
	}
	switch s.Level {
	case Self:
		where += fmt.Sprintf(" AND owner_id = $%d", len(args)+1)
		args = append(args, s.ActorID)
	case Space:
		where += fmt.Sprintf(" AND owner_space_id = $%d", len(args)+1)
		args = append(args, s.SpaceID)
	}
	return where, args
}
