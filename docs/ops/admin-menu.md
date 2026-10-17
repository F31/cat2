# 后台菜单（DB 驱动）

菜单不再由接口硬编码返回，而是启动时初始化到 `sys_admin_menu`，`GET /api/admin/v1/menus` 每次从 DB 读取并按 RBAC 动态过滤；前端按 `group/group_order` 渲染顶部一级、左侧二级。

## 表结构

| 字段 | 说明 |
| --- | --- |
| `key` | 稳定键（主键），如 `content.contents` |
| `label` | 菜单名 |
| `path` | 前端路由，如 `/contents` |
| `module` | 归属模块（用于按权限可见性过滤） |
| `group_key` / `group_label` | 一级菜单 key / 名称 |
| `group_order` | 一级菜单顺序 |
| `item_order` | 组内顺序 |
| `required_perm` | 额外权限点（空=只按模块可见性） |
| `super_only` | 仅超级管理员可见（如小程序设置） |
| `enabled` | 停用后接口不返回 |
| `updated_at` | 更新时间 |

## 初始化与修改

- 启动（`catcube migrate` / API 启动）执行 `seedAdminMenus`：幂等插入默认 25 项，**已存在的 key 不覆盖**，因此运营改过的标签/顺序/启停不会被重启重置。
- 新增菜单：插入一行并保证 `key/path` 唯一；重启后默认种子不会删除自定义行。
- 改标签/排序/停用（示例）：

```sql
UPDATE sys_admin_menu SET label='用户列表' WHERE key='identity.users';
UPDATE sys_admin_menu SET group_order=2, item_order=1 WHERE key='content.contents';
UPDATE sys_admin_menu SET enabled=false WHERE key='ops.exports';
```

- 删除默认菜单后，下次启动会被种子重新插入；如需永久隐藏请用 `enabled=false`。

## 接口过滤

`GET /api/admin/v1/menus`：

1. 读取 `enabled=true` 的菜单，按 `group_order,item_order` 排序；
2. 超级管理员（`idn_user.admin_role>0`）返回全部；
3. 其他后台用户：跳过 `super_only`；`required_perm` 不在其权限并集则跳过；`module` 不在可见模块内则跳过。

前端只消费接口 JSON，不再维护静态菜单；接口失败时才回退本地兜底菜单。

## 当前默认分组

概览 / 内容管理 / 交易与会员 / 圈子与立方 / 运营管理 / 权限管理 / 系统管理（详见 `docs/prd-trace` 与后台实际展示）。
