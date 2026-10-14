// Package statemachine 声明式状态机引擎：{from,event,to,actor,guard}。
// 非法转移在引擎层拒绝；统一写状态日志并发布事件。
package statemachine

import (
	"context"
	"fmt"
)

// Guard 转移守卫，返回 error 表示拒绝（Reason 驱动前端提示）。
type Guard func(ctx context.Context, actor string, entityID int64) error

// Transition 一条转移规则。
type Transition struct {
	From  string
	Event string
	To    string
	Actor string // 允许触发者角色（* 表示不限）
	Guard Guard
}

// Def 状态机定义（模块以声明式注册）。
type Def struct {
	Name        string
	States      []string
	Terminal    []string // 终态（只读保留，不物理删除）
	Transitions []Transition
}

// Log 状态流转日志。
type Log struct {
	Machine  string
	EntityID int64
	From     string
	Event    string
	To       string
	Actor    string
	At       int64
}

// IllegalTransitionError 非法转移。
type IllegalTransitionError struct {
	Machine string
	From    string
	Event   string
}

func (e *IllegalTransitionError) Error() string {
	return fmt.Sprintf("statemachine %s: 非法转移 %s --%s-->", e.Machine, e.From, e.Event)
}

// Engine 执行状态机。
type Engine struct {
	onChange func(l Log) // 写日志 + 发布事件钩子
}

func NewEngine(onChange func(Log)) *Engine { return &Engine{onChange: onChange} }

// Apply 校验并执行转移；非法转移返回 *IllegalTransitionError。
func (e *Engine) Apply(ctx context.Context, def Def, entityID int64, from, event, actor string) (string, error) {
	for _, t := range def.Transitions {
		if t.From == from && t.Event == event {
			if t.Actor != "" && t.Actor != "*" && t.Actor != actor {
				return "", fmt.Errorf("statemachine %s: 角色 %q 无权触发 %q", def.Name, actor, event)
			}
			if t.Guard != nil {
				if err := t.Guard(ctx, actor, entityID); err != nil {
					return "", err
				}
			}
			if e.onChange != nil {
				e.onChange(Log{Machine: def.Name, EntityID: entityID, From: from, Event: event, To: t.To, Actor: actor})
			}
			return t.To, nil
		}
	}
	return "", &IllegalTransitionError{Machine: def.Name, From: from, Event: event}
}

// Validate 校验定义合法性（状态闭环、终态无出边可警告等），供测试与启动自检。
func Validate(def Def) error {
	if def.Name == "" {
		return fmt.Errorf("statemachine: 缺名称")
	}
	states := map[string]bool{}
	for _, s := range def.States {
		states[s] = true
	}
	for _, t := range def.Transitions {
		if !states[t.From] || !states[t.To] {
			return fmt.Errorf("statemachine %s: 转移引用了未声明状态 %q/%q", def.Name, t.From, t.To)
		}
	}
	return nil
}
