package app

import (
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/menu"
	"context"
)

// defaultAdminMenus 首次初始化到 sys_admin_menu；之后以数据库为准。
// 层级：group（顶部一级）→ parent_key=” 的二级节点（分类或页面）→ parent_key=<二级 key> 的三级页面。
func defaultAdminMenus() []menu.Item {
	m := func(key, parent, label, path, module, group, groupLabel string, groupOrder, order int) menu.Item {
		return menu.Item{Key: key, ParentKey: parent, Label: label, Path: path, Module: module,
			Group: group, GroupLabel: groupLabel, GroupOrder: groupOrder, Order: order, Enabled: true}
	}
	content := func(key, parent, label, path, module string, order int) menu.Item {
		return m(key, parent, label, path, module, "content", "内容管理", 2, order)
	}
	trade := func(key, label, path, module string, order int) menu.Item {
		return m(key, "", label, path, module, "trade", "交易与会员", 3, order)
	}
	collab := func(key, label, path, module string, order int) menu.Item {
		return m(key, "", label, path, module, "collab", "圈子与立方", 4, order)
	}
	operation := func(key, parent, label, path, module string, order int) menu.Item {
		return m(key, parent, label, path, module, "operation", "运营管理", 5, order)
	}
	system := func(key, parent, label, path, module string, order int) menu.Item {
		return m(key, parent, label, path, module, "system", "系统管理", 6, order)
	}
	items := []menu.Item{
		// 内容管理 → 内容审核 / 内容配置
		content("content.review", "", "内容审核", "/m/content/review", "content", 1),
		content("content.contents", "content.review", "内容列表", "/contents", "content", 1),
		content("trust.reviews", "content.review", "审核队列", "/reviews", "trust", 2),
		content("verify.certs", "content.review", "认证审核", "/verify-certs", "verify", 3),
		content("verify.badges", "content.review", "认证标识", "/badges", "verify", 4),
		content("content.base", "", "内容配置", "/m/content/base", "content", 2),
		content("dict.taxonomy", "content.base", "主体类型与选项", "/dict", "dict", 1),
		content("profile.templates", "content.base", "画像字段模板", "/profile-templates", "profile", 2),

		// 交易与会员（直接二级页面）
		trade("trade.orders", "订单管理", "/orders", "trade", 1),
		trade("membership.members", "会员管理", "/members", "membership", 2),
		trade("ops.exports", "导出中心", "/exports", "ops", 3),

		// 圈子与立方
		collab("cube.cubes", "猫立方管理", "/cubes", "cube", 1),
		collab("circle.circles", "圈子管理", "/circles", "circle", 2),

		// 运营管理 → 渠道管理 / 运营工具
		operation("operation.channel", "", "渠道管理", "/m/operation/channel", "ops", 1),
		operation("channel.operators", "operation.channel", "运营商", "/channel-operators", "channel", 1),
		operation("channel.settlements", "operation.channel", "渠道结算", "/channel-settlements", "channel", 2),
		operation("ops.channel_settings", "operation.channel", "渠道与佣金", "/channel-settings", "ops", 3),
		operation("ops.membership_pricing", "operation.channel", "会员价格", "/membership-pricing", "ops", 4),
		operation("operation.tools", "", "运营工具", "/m/operation/tools", "ops", 2),
		operation("ops.governance", "operation.tools", "代操作", "/governance", "ops", 1),
		operation("ai.prompts", "operation.tools", "AI 提示词与用量", "/ai-prompts", "ai", 2),

		// 系统管理 → 权限管理 / 系统配置 / 运维工具
		system("system.authority", "", "权限管理", "/m/system/authority", "ops", 1),
		system("identity.users", "system.authority", "用户管理", "/users", "identity", 1),
		system("ops.rbac", "system.authority", "角色管理", "/roles", "ops", 2),
		system("system.config", "", "系统配置", "/m/system/config", "ops", 2),
		system("ops.system_settings", "system.config", "系统设置", "/system-settings", "ops", 1),
		system("ops.sms_settings", "system.config", "短信网关", "/sms-settings", "ops", 2),
		system("ops.ai_gateway_settings", "system.config", "AI 网关", "/ai-gateway-settings", "ops", 3),
		system("system.miniapp", "system.config", "小程序设置", "/miniapp-settings", "ops", 4),
		system("system.ops", "", "运维工具", "/m/system/ops", "ops", 3),
		system("ops.audits", "system.ops", "日志审计", "/audits", "ops", 1),
		system("ops.feature_flags", "system.ops", "特性开关", "/feature-flags", "ops", 2),
		system("ops.dead_letters", "system.ops", "死信队列", "/dead-letters", "ops", 3),
		system("notice.templates", "system.ops", "通知模板", "/templates", "notice", 4),
	}
	for i := range items {
		switch items[i].Key {
		case "ops.rbac":
			items[i].RequiredPerm = "ops:rbac:manage"
		case "ops.system_settings", "ops.sms_settings", "ops.ai_gateway_settings", "ops.feature_flags", "ops.channel_settings", "ops.membership_pricing":
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
	// 首页固定为登录后默认页，不占菜单项。
	if _, err := pool.Exec(ctx, `DELETE FROM sys_admin_menu WHERE key IN ('ops.dashboard','ops.settings')`); err != nil {
		return err
	}
	// 历史版：权限管理独立一级 → 系统管理；业务配置菜单移除。
	if _, err := pool.Exec(ctx, `
		UPDATE sys_admin_menu SET group_key='system', group_label='系统管理', group_order=6
		WHERE group_key='authority'`); err != nil {
		return err
	}
	// 结构迁移：为已知页面补齐二级分类 parent（已有运营调整的排序/标签保留）。
	parentOf := map[string]string{
		"content.contents": "content.review", "trust.reviews": "content.review",
		"verify.certs": "content.review", "verify.badges": "content.review",
		"dict.taxonomy": "content.base", "profile.templates": "content.base",
		"channel.operators": "operation.channel", "channel.settlements": "operation.channel",
		"ops.channel_settings": "operation.channel", "ops.membership_pricing": "operation.channel",
		"ops.governance": "operation.tools", "ai.prompts": "operation.tools",
		"identity.users": "system.authority", "ops.rbac": "system.authority",
		"ops.system_settings": "system.config", "system.miniapp": "system.config",
		"ops.audits": "system.ops", "ops.feature_flags": "system.ops",
		"ops.dead_letters": "system.ops", "notice.templates": "system.ops",
	}
	for key, parent := range parentOf {
		if _, err := pool.Exec(ctx, `UPDATE sys_admin_menu SET parent_key=$2 WHERE key=$1`, key, parent); err != nil {
			return err
		}
	}
	// 系统管理固定为第 6 个一级分组（兼容旧库遗留的 7）。
	if _, err := pool.Exec(ctx, `UPDATE sys_admin_menu SET group_order=6 WHERE group_key='system'`); err != nil {
		return err
	}
	// 系统配置子项顺序：系统设置 / 短信网关 / AI 网关 / 小程序设置。
	if _, err := pool.Exec(ctx, `
		UPDATE sys_admin_menu SET item_order=CASE key
			WHEN 'ops.system_settings' THEN 1
			WHEN 'ops.sms_settings' THEN 2
			WHEN 'ops.ai_gateway_settings' THEN 3
			WHEN 'system.miniapp' THEN 4
		END
		WHERE key IN ('ops.system_settings','ops.sms_settings','ops.ai_gateway_settings','system.miniapp')`); err != nil {
		return err
	}
	return menu.New(pool).Seed(ctx, defaultAdminMenus())
}
