package module

import (
	"strings"
	"testing"
)

type fakeModule struct {
	name string
	rank Rank
	deps []string
}

func (m fakeModule) Name() string              { return m.name }
func (m fakeModule) Rank() Rank                { return m.rank }
func (m fakeModule) DependsOn() []string       { return m.deps }
func (m fakeModule) Register(*Registry) error  { return nil }

func TestLoadTopo(t *testing.T) {
	mods := []Module{
		fakeModule{"notice", RankR6, []string{"identity"}},
		fakeModule{"identity", RankR0, []string{"dict"}},
		fakeModule{"dict", RankShared, nil},
	}
	ordered, err := Load(mods)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{ordered[0].Name(), ordered[1].Name(), ordered[2].Name()}
	if names[0] != "dict" || names[1] != "identity" || names[2] != "notice" {
		t.Fatalf("拓扑顺序错误: %v", names)
	}
}

func TestLoadMissingDepFails(t *testing.T) {
	mods := []Module{fakeModule{"cube", RankR3, []string{"content"}}} // content 未装载
	if _, err := Load(mods); err == nil || !strings.Contains(err.Error(), "缺失") {
		t.Fatalf("缺依赖应启动即失败: %v", err)
	}
}

func TestLoadCycleFails(t *testing.T) {
	mods := []Module{
		fakeModule{"a", RankShared, []string{"b"}},
		fakeModule{"b", RankShared, []string{"a"}},
	}
	if _, err := Load(mods); err == nil {
		t.Fatal("成环应失败")
	}
}

func TestSameRankDependencyRejected(t *testing.T) {
	mods := []Module{
		fakeModule{"msg", RankR1, []string{"trade"}},
		fakeModule{"trade", RankR1, nil},
	}
	if _, err := Load(mods); err == nil || !strings.Contains(err.Error(), "同阶") {
		t.Fatalf("同阶依赖应被拒绝: %v", err)
	}
}
