package statemachine

import (
	"context"
	"fmt"
	"sort"
)

// SelfCheck 定义自检：状态声明、转移合法、无重复转移、终态无出边。
// 返回全部问题（空=通过），供启动自检与表驱动测试。
func SelfCheck(def Def) []string {
	var issues []string
	if def.Name == "" {
		issues = append(issues, "缺名称")
	}
	states := map[string]bool{}
	for _, s := range def.States {
		if states[s] {
			issues = append(issues, fmt.Sprintf("状态 %q 重复声明", s))
		}
		states[s] = true
	}
	terminal := map[string]bool{}
	for _, s := range def.Terminal {
		if !states[s] {
			issues = append(issues, fmt.Sprintf("终态 %q 未在 States 声明", s))
		}
		terminal[s] = true
	}
	seen := map[string]bool{}
	for _, t := range def.Transitions {
		key := t.From + "|" + t.Event
		if seen[key] {
			issues = append(issues, fmt.Sprintf("重复转移 %s --%s-->", t.From, t.Event))
		}
		seen[key] = true
		if !states[t.From] || !states[t.To] {
			issues = append(issues, fmt.Sprintf("转移 %s --%s--> %s 引用未声明状态", t.From, t.Event, t.To))
		}
		if terminal[t.From] {
			issues = append(issues, fmt.Sprintf("终态 %q 存在出边（%s）", t.From, t.Event))
		}
		if t.Actor == "" {
			issues = append(issues, fmt.Sprintf("转移 %s --%s--> 未声明触发角色", t.From, t.Event))
		}
	}
	return issues
}

// Events 全部事件名（去重排序）。
func Events(def Def) []string {
	set := map[string]bool{}
	for _, t := range def.Transitions {
		set[t.Event] = true
	}
	out := make([]string, 0, len(set))
	for e := range set {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// Exhaustive 穷举校验：对「每个状态 × 每个事件」组合，合法转移必须成功且落到声明目标；
// 非法组合必须返回 *IllegalTransitionError。返回检查组合数与违规说明。
func Exhaustive(def Def) (int, []string) {
	engine := NewEngine(nil)
	ctx := context.Background()
	legal := map[string]Transition{}
	for _, t := range def.Transitions {
		legal[t.From+"|"+t.Event] = t
	}
	checked := 0
	var bad []string
	for _, from := range def.States {
		for _, ev := range Events(def) {
			checked++
			if t, ok := legal[from+"|"+ev]; ok {
				// 合法转移：以声明的触发角色执行
				to, err := engine.Apply(ctx, def, 1, from, ev, t.Actor)
				if err != nil {
					bad = append(bad, fmt.Sprintf("合法转移 %s --%s--> 被拒: %v", from, ev, err))
					continue
				}
				if to != t.To {
					bad = append(bad, fmt.Sprintf("转移 %s --%s--> 落点应为 %s，实际 %s", from, ev, t.To, to))
				}
				continue
			}
			to, err := engine.Apply(ctx, def, 1, from, ev, "*")
			if err == nil {
				bad = append(bad, fmt.Sprintf("非法转移 %s --%s--> 未被拒绝（落到 %s）", from, ev, to))
			}
			var ite *IllegalTransitionError
			if err != nil && !asIllegal(err, &ite) {
				bad = append(bad, fmt.Sprintf("非法转移 %s --%s--> 应返回 IllegalTransitionError，实际 %v", from, ev, err))
			}
		}
	}
	return checked, bad
}

func asIllegal(err error, target **IllegalTransitionError) bool {
	if e, ok := err.(*IllegalTransitionError); ok {
		*target = e
		return true
	}
	return false
}
