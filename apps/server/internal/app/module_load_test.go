package app

import (
	"strings"
	"testing"

	"catcube/internal/kernel/module"
	"catcube/internal/modules/content"
	"catcube/internal/modules/identity"
	"catcube/internal/modules/membership"
	"catcube/internal/modules/profile"
	"catcube/internal/modules/social"
	"catcube/internal/modules/trade"
	"catcube/internal/modules/trust"
	"catcube/internal/modules/verify"
	"catcube/internal/shared/dict"
	"catcube/internal/shared/media"
)

// 真实模块（nil 依赖）装载拓扑：dict/media 先于 identity/trust，Register 全通过。
func TestRealModulesLoad(t *testing.T) {
	mods := []module.Module{
		media.New(nil, nil),
		identity.New(identity.Deps{}),
		trust.New(trust.Deps{}),
		dict.New(nil),
		profile.New(profile.Deps{}),
		verify.New(verify.Deps{}),
		content.New(content.Deps{}),
		social.New(social.Deps{}),
		trade.New(trade.Deps{}),
		membership.New(membership.Deps{}),
	}
	ordered, err := module.Load(mods)
	if err != nil {
		t.Fatal(err)
	}
	order := map[string]int{}
	for i, m := range ordered {
		order[m.Name()] = i
	}
	if order["dict"] > order["identity"] {
		t.Fatalf("identity 依赖 dict，装载顺序错误: %v", ordered)
	}
	// 逐模块 Register 必须全通过（路由/能力/扩展点挂载不依赖 DB 连接）
	for _, m := range ordered {
		if err := m.Register(module.NewRegistry(m.Name())); err != nil {
			t.Fatalf("模块 %s Register 失败: %v", m.Name(), err)
		}
	}
}

// 缺依赖启动即失败：identity 依赖 dict，缺 dict 时 Load 必须报错。
func TestRealModulesMissingDepFails(t *testing.T) {
	mods := []module.Module{identity.New(identity.Deps{})}
	if _, err := module.Load(mods); err == nil || !strings.Contains(err.Error(), "缺失") {
		t.Fatalf("缺 dict 依赖应启动即失败: %v", err)
	}
}
