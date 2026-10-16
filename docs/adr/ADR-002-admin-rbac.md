# ADR-002 · 后台 RBAC 权限模型（采纳）

- 状态：**采纳**（p0 #13 已确认，2026-10-17）
- 决策范围：后台账号、角色与权限点模型（`p4-channel-admin` §3）
- 关联：`docs/prd-trace/decision-checklist-13.md` D1、`docs/adr/ADR-001-channel-data-model.md` §7、`openspec/changes/p4-channel-admin/tasks.md` §3

## 背景

p4 §3 原表述为「五内置角色种子 + 权限点矩阵」，易被误读为**角色—权限写死**。
经立项方确认：五角色与矩阵应作为**初始化默认值（seed）**，除超管外，角色权限必须**可配置**。本 ADR 固化该模型。

## 决策

1. **权限点唯一登记**：标识 `<module>:<resource>:<action>`，代码内集中登记（`contracts` + 各模块 `AdminSection`/`AuditPoint` 同源），
   命名**只增不改**；废弃用 `deprecated` 标记而非改名/删除。服务端为唯一权威，前端隐藏仅体验。

2. **关系数据化（N:M）**，`sys_` 内核表：
   - `sys_admin_user`：后台账号（与小程序用户 `idn_user` 分离）。
   - `sys_admin_role`：角色（`code/name`、`protected`、可选数据范围 `space_id`/`region_code`、`builtin`）。
   - `sys_admin_user_role`：账号 ↔ 角色（N:M，支持一人多角色，权限为并集）。
   - `sys_admin_role_permission`：角色 ↔ 权限点（N:M，**可配置**）。
   - 初始化种子：写入五内置角色（超管/运营/审核员/财务/渠道经理）+ `decision-checklist-13.md §2.2` 矩阵作为**默认权限**。

3. **超管固定，其余可配**：
   - 超管角色 `protected=true`：**隐含全部权限**，不可编辑/删除/降级；系统保证恒有至少一个超管（`catcube grant-admin` 兜底，防锁死）。
   - 非超管角色权限由持有 `ops:rbac:manage` 的账号（超管，或获授权的运营管理员）在后台配置。
   - **运营管理员不新增角色**：给「运营」或指定账号授予 `ops:rbac:manage` 即可（受第 4 条约束）。

4. **允许自定义角色**：超管可创建/删除 `builtin=false` 的角色；删除前校验无在用账号；内置角色可改权限但不可删除。

5. **防提权 / 防锁死（红线）**：
   - 反提权：编辑者只能授予「自身权限集的子集」，不得授予超出自身的权限；不得修改 `protected` 角色。
   - 防自锁：禁止移除最后一个具备 `ops:rbac:manage` 的账号/角色权限。
   - 违反上述一律 `denied` 记审计。

6. **审计与生效**：`ops:rbac:*`（角色增删、role_permission set、user_role assign、denied）全量审计；
   变更后使权限缓存/会话失效（角色权限版本号）；后台登录 `denied` 记审计（防爆破）。

7. **数据范围属角色配置（2026-10-17 细化）**：非超管角色带 `space_id`；渠道经理默认按区域过滤，且支持
   **多区域 + 含子区域**（`region_codes text[]` + `include_subtree bool`，而非单值 `region_code`）；服务端过滤为唯一权威。

8. **后台登录安全（2026-10-17 细化）**：
   - **连续 3 次登录失败 → 触发二次验证（短信，复用实名手机号）**；
   - **IP 白名单由超管在后台配置**（未配置=不限制）；
   - 后台登录 `denied` 记审计（防爆破）。

9. **双人复核清单可配（2026-10-17 细化）**：强制「复核 ≠ 执行」的操作清单默认按建议（退款/结算打款/会员人工调整/改价格比例/导出等），
   可由**运营管理员**在后台配置（`ops:policy:dual_control`，热生效）。

10. **账号模型（2026-10-17 建议）**：一期**复用平台账号 + RBAC**（不新建第二套账号体系）；安全靠 RBAC 最小权限、
    后台独立会话、失败锁定 + 短信二次验证、IP 白名单、双人复核。`sys_admin_user` 预留独立凭证字段，后续可演进为独立后台账号，无需重构。

11. **复用内核，不新增能力**：settings（默认值/灰度）、policy（反提权/防锁死/复核≠执行）、audit、statemachine、导出中心、AdminSection。

## 数据模型（草案）

```
sys_admin_user(id, user_id, account, status, created_at)            -- 一期关联平台用户；预留 credential_hash 演进独立账号
sys_admin_role(id, code, name, protected bool, builtin bool, space_id, region_codes text[], include_subtree bool, created_at)
sys_admin_user_role(user_id, role_id, PRIMARY KEY(user_id, role_id))
sys_admin_role_permission(role_id, perm, PRIMARY KEY(role_id, perm))   -- perm = <module>:<resource>:<action>
sys_admin_login(id, user_id, failed_count, locked_until, last_ip, created_at)  -- 失败锁定 + 二次验证状态
```

## 备选方案与否决理由

| 备选 | 否决理由 |
| --- | --- |
| 角色—权限硬编码在代码 | 无法运营期调整；与立项方要求不符 |
| 沿用 `idn_user.admin_role` 单级管理员 | 无最小权限、无审计、无法满足资金/审核互斥 |
| 超管也可被编辑 | 易锁死/提权，内控红线 |
| 新增「运营管理员」第六角色 | 冗余；以 `ops:rbac:manage` 权限点即可，且天然受反提权约束 |

## 影响

- **p4 §3**：新增四张 `sys_` 表（KernelTables）、五角色+矩阵种子、角色权限配置接口与后台页、反提权/防锁死策略、表驱动与负向测试。
- **测试**：默认矩阵生效；改权限即时生效；超管不可改；反提权 denied；防自锁；自定义角色创建/删除；数据范围过滤。
- **不变**：`requireAdmin` 过渡实现保留至 §3 上线，作为引导入口（`catcube grant-admin`）。
