package app

import (
	"testing"

	"catcube/internal/kernel/module"
)

type fakeModule struct {
	name string
	rank module.Rank
	deps []string
}

func (f fakeModule) Name() string                    { return f.name }
func (f fakeModule) Rank() module.Rank               { return f.rank }
func (f fakeModule) DependsOn() []string             { return f.deps }
func (f fakeModule) Register(*module.Registry) error { return nil }

func TestSelectModules(t *testing.T) {
	all := []module.Module{
		fakeModule{"dict", module.RankShared, nil},
		fakeModule{"identity", module.RankR0, nil},
		fakeModule{"content", module.RankR2, []string{"dict", "identity"}},
		fakeModule{"cube", module.RankR3, []string{"identity", "content"}},
		fakeModule{"ops", module.RankR1, []string{"identity"}},
	}
	// 空=全部
	if got, err := selectModules(all, nil); err != nil || len(got) != len(all) {
		t.Fatalf("空开关应全部装载: %v %d", err, len(got))
	}
	// 仅启用 content（含依赖 identity）→ shared+ops+identity+content
	got, err := selectModules(all, []string{"content"})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, m := range got {
		names[m.Name()] = true
	}
	if !names["identity"] || !names["content"] || !names["dict"] || !names["ops"] || names["cube"] {
		t.Fatalf("过滤结果不符: %+v", names)
	}
	// 传递补齐：仅启用 cube → 自动补齐 identity/content（cube 依赖）
	got, err = selectModules(all, []string{"cube"})
	if err != nil {
		t.Fatal(err)
	}
	names = map[string]bool{}
	for _, m := range got {
		names[m.Name()] = true
	}
	if !names["cube"] || !names["identity"] || !names["content"] || !names["dict"] || names["ops"] == false {
		t.Fatalf("依赖应传递补齐: %+v", names)
	}
	// 未知模块名 → 报错
	if _, err := selectModules(all, []string{"nope"}); err == nil {
		t.Fatal("未知模块应报错")
	}
}
