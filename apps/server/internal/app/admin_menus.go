package app

import (
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/menu"
	"context"
)

// defaultAdminMenus 首次初始化到 sys_admin_menu；之后以数据库为准（运营可改 tag/顺序/启停）。
func defaultAdminMenus() []menu.Item {
	m := func(key, label, path, module, group, groupLabel string, groupOrder, order int) menu.Item {
		return menu.Item{Key: key, Label: label, Path: path, Module: module, Group: group,
			GroupLabel: groupLabel, GroupOrder: groupOrder, Order: order, Enabled: true}
	}
	overview := func(key, label, path, module string, order int) menu.Item {
		return m(key, label, path, module, "overview", "概览", 1, order)
	}
	content := func(key, label, path, module string, order int) menu.Item {
		return m(key, label, path, module, "content", "内容管理", 2, order)
	}
	trade := func(key, label, path, module string, order int) menu.Item {
		return m(key, label, path, module, "trade", "交易与会员", 3, order)
	}
	collab := func(key, label, path, module string, order int) menu.Item {
		return m(key, label, path, module, "collab", "圈子与立方", 4, order)
	}
	operation := func(key, label, path, module string, order int) menu.Item {
		return m(key, label, path, module, "operation", "运营管理", 5, order)
	}
	authority := func(key, label, path, module string, order int) menu.Item {
		return m(key, label, path, module, "authority", "权限管理", 6, order)
	}
	system := func(key, label, path, module string, order int) menu.Item {
		return m(key, label, path, module, "system", "系统管理", 7, order)
	}
	items := []menu.Item{
		overview("ops.dashboard", "经营概览", "/dashboard", "ops", 1),

		content("content.contents", "内容列表", "/contents", "content", 1),
		content("trust.reviews", "审核队列", "/reviews", "trust", 2),
		content("verify.certs", "认证审核", "/verify-certs", "verify", 3),
		content("verify.badges", "认证标识", "/badges", "verify", 4),
		content("dict.taxonomy", "主体类型与选项", "/dict", "dict", 5),
		content("profile.templates", "画像字段模板", "/profile-templates", "profile", 6),

		trade("trade.orders", "订单管理", "/orders", "trade", 1),
		trade("membership.members", "会员管理", "/members", "membership", 2),
		trade("ops.exports", "导出中心", "/exports", "ops", 3),

		collab("cube.cubes", "猫立方管理", "/cubes", "cube", 1),
		collab("circle.circles", "圈子管理", "/circles", "circle", 2),

		operation("channel.operators", "运营商", "/channel-operators", "channel", 1),
		operation("channel.settlements", "渠道结算", "/channel-settlements", "channel", 2),
		operation("ops.governance", "代操作", "/governance", "ops", 3),
		operation("ai.prompts", "AI 提示词与用量", "/ai-prompts", "ai", 4),

		authority("identity.users", "用户管理", "/users", "identity", 1),
		authority("ops.rbac", "角色管理", "/roles", "ops", 2),

		system("ops.system_settings", "系统设置", "/system-settings", "ops", 1),
		system("ops.audits", "日志审计", "/audits", "ops", 2),
		system("ops.feature_flags", "特性开关", "/feature-flags", "ops", 3),
		system("ops.dead_letters", "死信队列", "/dead-letters", "ops", 4),
		system("notice.templates", "通知模板", "/templates", "notice", 5),
		system("ops.settings", "业务配置", "/settings", "ops", 6),
		system("system.miniapp", "小程序设置", "/miniapp-settings", "ops", 7),
	}
	// 权限点/超管限制（与旧菜单过滤规则一致；其余按模块可见性过滤）
	for i := range items {
		switch items[i].Key {
		case "ops.rbac":
			items[i].RequiredPerm = "ops:rbac:manage"
		case "ops.system_settings", "ops.settings", "ops.feature_flags":
			items[i].RequiredPerm = "ops:settings:read"
		case "system.miniapp":
			items[i].SuperOnly = true
		}
	}
	return items
}

func seedAdminMenus(ctx context.Context, pool *db.Pool) error {
	if _, err := pool.Exec(ctx, menu.Schema); err != nil {
		return err
	}
	return menu.New(pool).Seed(ctx, defaultAdminMenus())
}
