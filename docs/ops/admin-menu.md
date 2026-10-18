# 后台菜单（DB 驱动）

菜单不再由接口硬编码返回，而是启动时初始化到 `sys_admin_menu`，`GET /api/admin/v1/menus` 每次从 DB 读取并按 RBAC 动态过滤；前端按 `group/group_order` 渲染顶部一级、左侧二级。

## 表结构

| 字段 | 说明 |
| --- | --- |
| `key` | 稳定键（主键），如 `content.contents` |
| `parent_key` | 上级菜单 key；空=一级分组直属（二级节点） |
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

- 启动（`catcube migrate` / API 启动）执行 `seedAdminMenus`：幂等插入默认菜单（25 个页面 + 7 个分类节点），**已存在的 key 不覆盖**，因此运营改过的标签/顺序/启停不会被重启重置。
- 层级：`group_key`（顶部一级）→ `parent_key=''`（左侧二级，页面或分类）→ `parent_key=<二级 key>`（左侧三级页面）。分类节点使用 `/m/...` 占位路径，不参与路由跳转。
- 新增菜单：插入一行并保证 `key/path` 唯一；重启后默认种子不会删除自定义行。
- 改标签/排序/停用（示例）：

```sql
UPDATE sys_admin_menu SET label='用户列表' WHERE key='identity.users';
UPDATE sys_admin_menu SET group_order=2, item_order=1 WHERE key='content.contents';
UPDATE sys_admin_menu SET enabled=false WHERE key='ops.exports';
```

- 删除默认菜单后，下次启动会被种子重新插入；如需永久隐藏请用 `enabled=false`。

## 接口过滤

`GET /api/admin/v1/menus`（可带 `?group=operation`）：

1. 读取 `enabled=true` 的菜单，按 `group_order,item_order` 排序；
2. 超级管理员（`idn_user.admin_role>0`）返回全部；
3. 其他后台用户：跳过 `super_only`；`required_perm` 不在其权限并集则跳过；页面按 `module` 可见性过滤；分类节点按子节点存活情况剪枝；
4. `?group=xxx` 仅返回该一级分组下的二/三级菜单（`groups` 仍返回可见一级列表）。

前端点击顶部一级菜单时按需请求该分组；左侧展示树形二/三级菜单，三级默认折叠。接口失败时才回退本地兜底菜单。

## 当前默认分组

首页（`/dashboard`）不占菜单，登录后默认进入，点左上角「猫平方管理平台」回到首页；其余一级菜单：内容管理 / 交易与会员 / 广场 / 运营管理 / 系统管理。

三级结构示例：
- 系统管理 → 权限管理 → 用户管理/角色管理
- 系统管理 → 系统配置 → 系统设置/小程序设置
- 系统管理 → 运维工具 → 日志审计/特性开关/死信队列/通知模板
- 运营管理 → 渠道管理 → 运营商/渠道结算/渠道与佣金/会员价格
- 运营管理 → 运营工具 → 代操作/AI 提示词与用量
- 内容管理 → 内容审核 / 内容配置 → 对应页面
