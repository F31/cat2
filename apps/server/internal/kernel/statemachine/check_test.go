package statemachine

import (
	"context"
	"testing"
)

func sampleDef() Def {
	return Def{
		Name:     "demo",
		States:   []string{"a", "b", "c"},
		Terminal: []string{"c"},
		Transitions: []Transition{
			{From: "a", Event: "go", To: "b", Actor: "user"},
			{From: "b", Event: "done", To: "c", Actor: "admin"},
			{From: "b", Event: "back", To: "a", Actor: "user"},
		},
	}
}

func TestSelfCheck(t *testing.T) {
	if issues := SelfCheck(sampleDef()); len(issues) != 0 {
		t.Fatalf("样例定义应通过: %v", issues)
	}
	bad := sampleDef()
	bad.Terminal = append(bad.Terminal, "c") // 终态出边
	bad.Transitions = append(bad.Transitions, Transition{From: "c", Event: "reopen", To: "a", Actor: "admin"})
	if issues := SelfCheck(bad); len(issues) == 0 {
		t.Fatal("终态出边应被检出")
	}
}

func TestExhaustive(t *testing.T) {
	checked, bad := Exhaustive(sampleDef())
	if len(bad) != 0 {
		t.Fatalf("样例应无违规: %v", bad)
	}
	if checked != 3*3 {
		t.Fatalf("应穷举 9 组，实际 %d", checked)
	}
	// 不存在的转移必须被拒（引擎行为一致性）
	engine := NewEngine(nil)
	if _, err := engine.Apply(context.Background(), sampleDef(), 1, "a", "done", "*"); err == nil {
		t.Fatal("a--done--> 应非法")
	}
}
