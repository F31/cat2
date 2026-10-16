package rbac

import "testing"

// 权限点注册表唯一且默认矩阵只引用已登记权限点（防拼写漂移）。
func TestRegistryAndDefaultMatrix(t *testing.T) {
	reg := Registry()
	seen := map[string]bool{}
	for _, d := range reg {
		if d.Perm == "" || d.Label == "" {
			t.Fatalf("权限点定义不完整: %+v", d)
		}
		if seen[d.Perm] {
			t.Fatalf("权限点重复: %s", d.Perm)
		}
		seen[d.Perm] = true
	}
	for role, perms := range defaultPerms {
		for _, p := range perms {
			if !seen[p] {
				t.Fatalf("角色 %s 引用了未登记权限点: %s", role, p)
			}
		}
	}
	// 超管不在默认矩阵（隐含全部）
	if _, ok := defaultPerms[RoleSuper]; ok {
		t.Fatal("超管不应出现在默认权限矩阵（隐含全部）")
	}
}

// 内置角色含受保护超管。
func TestBuiltinRoles(t *testing.T) {
	found := false
	for _, r := range builtinRoles {
		if r.Code == RoleSuper {
			found = r.Protected && r.Builtin
		}
	}
	if !found {
		t.Fatal("超管应为受保护内置角色")
	}
}

// 菜单可见模块：按权限名前缀推导。
func TestVisibleModules(t *testing.T) {
	mods := VisibleModules([]string{"content:moderation:decide", "ops:dashboard:read", "bad"})
	if !mods["content"] || !mods["ops"] {
		t.Fatalf("应可见 content/ops: %v", mods)
	}
	if mods["trade"] {
		t.Fatalf("不应可见 trade: %v", mods)
	}
}

// 数据范围：多区域 + 含子区域（D5/§3.11）。
func TestRegionAllowed(t *testing.T) {
	if !RegionAllowed(nil, false, "110100") {
		t.Fatal("未限定区域应不限")
	}
	if !RegionAllowed([]string{"310000", "330000"}, false, "330000") {
		t.Fatal("多区域精确应命中")
	}
	if RegionAllowed([]string{"310000"}, false, "310100") {
		t.Fatal("不含子区域时市级不应命中省级限定")
	}
	if !RegionAllowed([]string{"310000"}, true, "310100") || !RegionAllowed([]string{"310000"}, true, "310101") {
		t.Fatal("含子区域应命中省市县")
	}
	if RegionAllowed([]string{"310000"}, true, "320000") {
		t.Fatal("不应越界到其它省")
	}
	if !RegionAllowed([]string{"310100"}, true, "310101") {
		t.Fatal("市级含子区域应命中区县")
	}
	if RegionAllowed([]string{"310100"}, true, "310200") {
		t.Fatal("不应命中非子区域")
	}
}
