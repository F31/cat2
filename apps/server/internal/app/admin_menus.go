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
	system := func(key, label, path, module string, order int) menu.Item {
		return m(key, label, path, module, "system", "系统管理", 6, order)
	}
	items := []menu.Item{
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

		system("identity.users", "用户管理", "/users", "identity", 1),
		system("ops.rbac", "角色管理", "/roles", "ops", 2),
		system("ops.system_settings", "系统设置", "/system-settings", "ops", 3),
		system("ops.audits", "日志审计", "/audits", "ops", 4),
		system("ops.feature_flags", "特性开关", "/feature-flags", "ops", 5),
		system("ops.dead_letters", "死信队列", "/dead-letters", "ops", 6),
		system("notice.templates", "通知模板", "/templates", "notice", 7),
		system("ops.settings", "业务配置", "/settings", "ops", 8),
		system("system.miniapp", "小程序设置", "/miniapp-settings", "ops", 9),
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
	// 首页固定为登录后默认页，不占菜单项（清理历史种子）。
	if _, err := pool.Exec(ctx, `DELETE FROM sys_admin_menu WHERE key='ops.dashboard'`); err != nil {
		return err
	}
	// 历史版本：权限管理为独立一级菜单 → 合并到系统管理（仅迁移一次）。
	var legacy bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_admin_menu WHERE group_key='authority')`).Scan(&legacy); err != nil {
		return err
	}
	if legacy {
		if _, err := pool.Exec(ctx, `UPDATE sys_admin_menu SET item_order=item_order+2 WHERE group_key='system'`); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `
			UPDATE sys_admin_menu
			SET group_key='system', group_label='系统管理', group_order=6,
				item_order=CASE key WHEN 'identity.users' THEN 1 WHEN 'ops.rbac' THEN 2 ELSE item_order END
			WHERE group_key='authority'`); err != nil {
			return err
		}
	}
	// 系统管理固定为第 6 个一级分组（兼容旧库遗留的 7）。
	if _, err := pool.Exec(ctx, `UPDATE sys_admin_menu SET group_order=6 WHERE group_key='system'`); err != nil {
		return err
	}
	return menu.New(pool).Seed(ctx, defaultAdminMenus())
}
