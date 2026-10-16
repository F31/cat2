# p4-channel-admin · 任务清单

> 前置：p0 待确认 #13（后台五角色划分）已于 **2026-10-17 书面确认**（含 RBAC 可配置修正，见 `docs/adr/ADR-002-admin-rbac.md`），本变更开工前置已解除。
>
> **预研已就绪（不触发门禁）**：数据模型/契约/状态机/复用点与待决策清单见 `docs/modules/channel-预研.md`，
> 技术决策见 `docs/adr/ADR-001-channel-data-model.md`（**采纳**）与 `docs/adr/ADR-002-admin-rbac.md`（**采纳**）。结论：**无新增内核能力需求**，
> 可按 §1 → §2 → §3 顺序开工（§1/§2 与 §3 可并行）。

## 1. channel：运营商移动端身份（小程序）

- [ ] 1.1 迁移：`chn_operator`、`chn_guarantee`、`chn_commission`、`chn_settlement`、`chn_annual_target`、`chn_attribution`（详细设计 4.2.18 附）
- [ ] 1.2 红线进 schema：`chn_operator.referrer_operator_id` 单列、无上级字段、不能自建下级；SQL 层与 contract 均无多级结构
- [ ] 1.3 运营商身份卡（个人中心显示身份与所属区域）；区域推广专属链接/邀请码
- [ ] 1.4 归属关系记录（`chn_attribution`：用户注册来源绑定运营商）
- [ ] 1.5 佣金明细、提现申请、业绩概览（本月新增付费会员/佣金合计/保底任务完成度）
- [ ] 1.6 状态机 #16 运营商（申请中→已开通→已停用，停用须重新申请）+ 表驱动测试

## 2. channel：佣金与结算（后台）

- [ ] 2.1 开户审核：区域查重一地一家；代理费首年 3 万/次年 2 万收款登记（SettingsSchema **动态可配**，权限点 `channel:operator_fee:set`）；保证金 1 万可退台账（`chn_guarantee` 预收款，不与收入混记，退款走台账）
- [ ] 2.2 佣金**规则配置（范围+比例）**：SettingsSchema `channel.commission.*` 初始值（首单 30%/续费 10%；含会员订阅、不含立方分摊与保证金），**运行期动态可配（热生效）**；权限点 `channel:commission_rule:set` 默认可授予运营管理员（`decision-checklist-13.md` D2）
- [ ] 2.3 订阅 `OrderPaid`、`MemberActivated`：按**计佣时配置**生成 `chn_commission` 流水并写 `ratio_snapshot`（含范围与比例的配置版本，后续变更不影响历史流水）
- [ ] 2.4 月结结算单状态机 #17（待结算→已结算→已打款，打款不可撤销）：生成→财务复核→对公打款→留凭证（`voucher_ref`→media）；**复核与执行不得同一人**（内核策略强制互斥，违反 denied 记审计）
- [ ] 2.5 保底任务跟踪：12 个月 ≥50 付费会员或 ≥3 落地项目；进度与预警（`chn_annual_target`）
- [ ] 2.6 归属纠错审批留痕（`chn_attribution.correct_approval`）
- [ ] 2.7 对账导出（按运营商/区域/时间）走导出中心
- [ ] 2.8 事件：`CommissionAccrued`、`SettlementPaid`、`OperatorStatusChanged` Schema 与发布
- [ ] 2.9 contract：`OperatorOf`、`CommissionOf`、`SettlementApprove/Pay`

## 3. 后台 RBAC 与权限点

- [x] 3.1 迁移四张 `sys_admin_*` 表（`kernel/rbac.Schema` 并入 `pg.KernelTables`）；五内置角色 + §2.2 矩阵**默认种子**（`rbac.Seed`，角色首建时写默认权限，此后可配）；`space_id/region_code`、`protected`、`builtin` 齐备
- [◐] 3.2 权限点命名 `<module>:<resource>:<action>` 与注册表 `rbac.Registry()`、`requirePerm` 守卫、菜单级过滤（`/menus` 按模块+`ops.rbac` 特判）已落地；**待办**：为各模块 Admin 路由批量标注所需权限点（action 级全覆盖）
- [x] 3.3 服务端唯一权威（`requireAdmin`/`requirePerm`；403 记 `admin:access:denied`/`admin:perm:denied` 审计，`TestIntegrationAdminRBAC`）
- [ ] 3.4 后台账号强制二次验证；敏感操作（退款/封禁/改价格比例/导出/结算）二次确认 + 审计
- [ ] 3.5 内核 policy「复核≠执行」互斥规则（同一结算单/退款单操作人≠复核人）
- [◐] 3.6 默认矩阵与负向用例已覆盖（`TestIntegrationAdminRBAC`：默认矩阵、反提权 denied、超管不可改、内置不可删、自定义角色增删）；**待办**：五角色×全权限点的完整表驱动矩阵用例
- [◐] 3.7 菜单按 RBAC 过滤已落地（`/menus` 按模块过滤，`ops.rbac` 仅 `ops:rbac:manage` 可见）；**待办**：角色 `space_id/region_code` 数据范围过滤（随 p4 §1/§2 渠道数据接入）
- [x] 3.8 角色权限可配置：后台「角色与权限」页（`apps/admin-web/src/pages/Roles.tsx`）+ 接口（permissions/roles/roles/{code}/permissions/admin-users/{id}/roles）；自定义角色（内置不可删）；改动即时生效（无缓存直接查库）并审计 `ops:rbac:*`
- [x] 3.9 RBAC 红线：超管 `protected` 不可编辑/降级/删除；反提权（`SetRolePerms`/`AssignRoles` 只能授予自身子集）；防锁死（保留 `ops:rbac:manage` 持有者/超管）；违反记 `ops:rbac:*` denied 审计
- [◐] 3.10 测试：`kernel/rbac` 单测 + `TestIntegrationAdminRBAC`（默认矩阵/可配/超管保护/反提权/自定义角色/内置不可删/审计）；**待办**：数据范围（space_id/region_code）过滤用例

## 4. 治理分区 governance/

- [x] 4.1 用户 360：`GET /api/admin/v1/users/{id}/360`（bff 只读聚合 8 区：identity/profile/verify（认证+归属+组织管理员）/membership（会员+子账号）/content/cube（含角色）/circle（含角色）/trust（受限状态）），每区标注来源模块；后台硬门槛（非管理员 403）；admin-web「用户 360」页；集成测试 `TestIntegrationUser360`。**trade 订单明细区待补**（facade 需增按用户查询）
- [x] 4.2 检索筛选：`GET /api/admin/v1/governance/users`（昵称模糊、**手机号盲索引精确**、状态、实名、身份类型、会员等级（membership 交集）、注册时间范围、分页）；结果标注会员等级与生效态；导出走导出中心（含 `export:create` 审计）；admin-web 用户页筛选栏；集成测试覆盖（含非管理员 403）
- [x] 4.3 封禁与限制：封禁/解禁（identity `ban/unban`，二次确认 + 审计）；**限制发布** `POST /api/admin/v1/users/{id}/restrict`（days>0 限制、days=0 解除，必须给原因，审计点 `trust:user:restrict`）；**内容发布门禁落地**：content `Publish/Submit` 校验 `publish.allowed` 能力，限制期返回 `110006`/429；circle 沿用同一能力；admin-web 用户页「限制/解除」二次确认
- [x] 4.4 代操作清单（除渠道相关与双人复核）：**已落地** 代处理超期核验（`GET /api/admin/v1/affiliations/overdue` + `POST /affiliations/{id}/resolve`）、重置组织管理员、转移企业版主账号、强制解除子账号、履约手动补偿、猫立方封禁/解散、圈子解散、会员人工调整（单人、含前后值审计）、定向公告、死信重放/任务重跑（`GET/POST /api/admin/v1/jobs[/{name}/run]`）；admin-web「代操作」页。**待办**：归属纠错审批（channel，#13）与「会员/资金类双人复核」（RBAC，#13）
- [x] 4.5 二次确认与全量审计：admin-web 代操作均需填写原因（审计强制）、危险操作二次确认；服务端统一 `audit.Change{Before,After}.Snapshot(reason)` 写入 detail（含脱敏）；敏感动词路由由 `TestSensitiveRoutesHaveAuditPoints` 强制声明审计点；集成测试 `TestIntegrationAdminOperations` 断言快照存在

## 5. 审计体系落地

- [x] 5.1 审计装饰器与自动埋点：`audit.Decorator.Decorate`（按 4xx/403 自动记 denied/failed，`X-Request-ID`+IP 入审计）、`audit.WithActor`/`ClientIP`/`RequestIDOf`；审计与业务同事务沿用现有 `Record(ctx, tx, e)`；写失败不阻断业务（调用方 `_ =`）并计入 `catcube_audit_total`/`catcube_audit_denied_total`
- [x] 5.2 审计脱敏：`audit.SanitizeDetail`（键名命中即掩码：手机号/证件号/姓名/密钥类整体 redacted；值内嵌手机号/证件号一并抹除），递归含嵌套与数组；单测 `TestSanitizeDetail` + 查询接口**二次脱敏兜底**（存量数据）
- [x] 5.3 AuditPoint 注册表：`module.Registry.AuditPoints` + `audit.Registry`（含 `Path`/`Module`）；**CI 校验** `TestSensitiveRoutesHaveAuditPoints`——敏感动词路由（refund/ban/takedown/dissolve/transfer/export/replay/resolve/settings/feature-flags）的写接口必须声明审计点（读接口豁免）
- [ ] 5.4 17 模块审计点清单逐一落地（详细设计 11.4 全表）
- [x] 5.5 审计页：`GET /api/admin/v1/audits`（actor/action/result/target_ref/时间/limit 五维筛选）+ `/audits/{id}`（含 hash/prev_hash 与快照）；admin-web「审计轨迹」页（筛选 + target 反查 + 详情抽屉快照查看）+ audit detail 二次脱敏
- [x] 5.6 安全告警：`ops.security_monitor`（5 分钟 denied 超阈 → `audit_denied_surge`，阈值 `audit.denied_surge_threshold` 可配）+ 指标 `catcube_audit_denied_5m`；联系方式阈值告警（`contact_scrape_suspected`）与审计断链（`audit_chain_broken`）此前已完成；登录失败锁定激增待 identity 侧补指标
- [x] 5.7 留存归档：设置 `audit.retention_days`（默认 180，资金/合规建议 ≥1095，可配）；`ops.audit_archive` 任务超期行标记 `archived_at` 并写 `sys_audit_archive` 边界哈希（不物理删除，链可续验离线校验）；查询返回 `archived` 标记与指标 `catcube_audit_archived_total`
- [x] 5.8 审计覆盖测试：`TestIntegrationAuditGovernance`（写审计→五维查询→详情哈希链→脱敏兜底→denied 告警→留存归档全链路）；敏感动词路由声明校验见 5.3

## 6. verify 认证全类型

- [x] 6.1 迁移补齐：`vfy_affiliation`（一人多单位：role/is_primary/proof/status/reviewer/SLA 可查）、`vfy_org_admin`（org/user/status/invited_by/confirmed_at），索引齐备
- [x] 6.2 五类认证：`CertRealname=1`（实时二要素）+ 2~5（个人/个人企业身份/企业/机构）经 `CertifyFor` 提交，材料必填且 ≤3 项；`reviewer_type` 区分平台（1）与企业（2，个人企业身份须带 `target_org_id`）；平台审核 `POST /api/admin/v1/certs/{id}/review`、企业审核 `POST /api/mp/v1/certs/{id}/review`
- [x] 6.3 状态机 #3/#4：`verify.cert`（待审核→已通过/已驳回；驳回可重提）、`verify.affiliation`（待核验→已核验/已驳回；已核验可撤销、驳回可重申请）注册进装配根，纳入 `TestIntegrationStateMachineTable` 穷举校验；服务层在提交（仅驳回后重提才转移）与审核/核验时经引擎校验；集成测试 `TestIntegrationVerifyStateMachines`（驳回→重提→重复处理 404→通过授标）
- [x] 6.4 身份核验：`GET /api/mp/v1/affiliations/mine`（含待核验状态驱动置灰/红框）、`POST /affiliations/{id}/verify`（组织管理员，**先到先处理**：重复处理返回 404/100006）、`POST /api/admin/v1/affiliations/{id}/resolve` 平台代处理（超期 `OrphanAffiliations` 查询）、`POST /affiliations/{id}/primary` 唯一主单位；事件 `verify.AffiliationVerified.v1`
- [x] 6.5 组织管理员：企业/机构认证通过时申请人自动成为首任管理员（`UpsertActiveOrgAdmin`）；邀请需对方确认（`POST /orgs/{id}/admins` → `POST /org-admin-invites/{id}/confirm`）；平台重置/移除 `POST /api/admin/v1/orgs/{id}/admins/reset`；事件 `verify.OrgAdminChanged.v1`
- [x] 6.6 一人多单位：`vfy_affiliation` 一对多（user+org 唯一）；状态驱动置灰（0 待核验）/红框（1 已核验）；`is_primary` 唯一主单位由 `SetPrimary` 维护
- [x] 6.7 Badge 机制：`vfy_badge_def`/`vfy_badge_grant` 输出 {code,label,level,style}；后台 `GET /api/admin/v1/badges`、`PUT /api/admin/v1/badges/{code}`（新增标识=加配置+前端样式，不改接口）、`POST /api/admin/v1/badges/grant`（授予/撤销，审计点 `verify:badge:config`/`verify:badge:grant`）；admin-web「认证标识」页；集成测试覆盖
- [x] 6.8 事件与不可改：认证材料不可编辑（重复提交为新的待审记录）；`verify.AffiliationVerified.v1` / `verify.OrgAdminChanged.v1` 事件 Schema 已入契约并生成（`contracts/events/`），notice 模板同步
- [x] 6.9 小程序认证页：`pages/verify/index` 重构为「实名 + 四类认证（材料 ≤3，个人企业身份选企业）+ 单位归属（一人多单位，核验后红框/未核验置灰、设主单位）+ 组织管理员待核验（红点，先到先处理）」；api-client 补 8 个方法与 `GET /affiliations/pending`

## 7. 统计看板与导出中心

- [x] 7.1 核心指标：`GET /api/admin/v1/dashboard`（用户/新增/实名、内容展示中、搜索/匹配点击/联系发起（埋点）、在期会员、今日支付单与 GMV、活跃圈/立方、审核积压与超 SLA、死信、AI 调用/降级）；实时聚合、不在业务表写统计；口径见 `docs/ops/dashboards.md`；admin-web 概览页展示
- [x] 7.2 埋点：`contracts/telemetry/README.md` 登记 8 个漏斗事件与属性；`tlm_event` 独立前缀表；`POST /api/mp/v1/telemetry`（≤50/次、仅登记事件入库、props 白名单 + 手机号/证件号形态剔除、失败不阻断）；指标 `catcube_telemetry_accepted_total`/`rejected_total`；集成测试覆盖
- [x] 7.3 看板：`dashboard` 四分组（平台/协作/资金/安全）+ `/dashboard/funnel`（注册→画像→匹配点击→联系发起→支付，去重用户与转化率，支付含业务表兜底口径）；admin-web 概览页（核心指标 + 漏斗条 + 协作/资金/安全分组）。**渠道与运营商移动端看板待 channel 模块（#13）**
- [x] 7.4 导出中心：`sys_export_task` + `ops.export_runner`（异步生成 CSV 到对象存储）；模板 对账/订单/用户（脱敏）/审核记录（结算模板待渠道开通）；**带水印**行 + 7 天有效期 + `export:create`/`export:download` 审计点；**逐字段敏感白名单校验**（文本列禁手机号/证件号，纯数字列豁免）；admin-web「导出中心」；集成测试 `TestIntegrationExportAndTelemetryDashboard`

## 8. admin-web 页面完成

- [ ] 8.1 features 全部页面：dict（主体类型/选项/字段模板）、content（内容/产品库入库三步）、trust（审核队列/举报/敏感词/违规记录）、verify（认证审核）、cube/circle（管理/封禁/解散）、trade（订单/退款/对账）、membership（会员/子账号转移/价格配置）、channel（渠道管理全套）、discovery（匹配参数与权重带版本）、notice（公告/模板）、system（配置中心 SettingsSchema 自动渲染/特性开关/审计日志/后台账号）
- [ ] 8.2 配置中心不手写页面：由 SettingsSchema 自动渲染；新增配置项不写前端代码
- [ ] 8.3 后台共享组件：SettingsForm、ReviewQueue、AuditTrail、StateTimeline、DataTable

## 9. 测试与门禁

- [ ] 9.1 权限矩阵补全后台格：五角色 × 权限点全量表驱动用例
- [ ] 9.2 状态机测试：#3、#4、#16、#17 全表
- [ ] 9.3 结算闭环测试：佣金计提 → 月结生成 → 财务复核 → 打款 → 凭证；复核=执行 denied 用例
- [ ] 9.4 审计覆盖面测试（tests/audit）与 CI 审计点校验
- [ ] 9.5 契约测试：admin.yaml 全量路径、channel/verify 事件 Schema

## 10. 退出标准

- [ ] 10.1 对账结算闭环跑通（结算单生成→财务复核→对公打款→留凭证，复核≠执行机制生效）
- [ ] 10.2 权限矩阵 + 审计覆盖面测试全绿
- [ ] 10.3 认证全类型（五项）与身份核验链路走查通过
