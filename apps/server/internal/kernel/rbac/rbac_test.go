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
