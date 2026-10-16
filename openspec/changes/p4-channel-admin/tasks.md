# p4-channel-admin · 任务清单

> 前置阻塞：p0 待确认 #13（后台五角色划分）已书面确认，否则本变更不开工。
>
> **预研已就绪（不触发门禁）**：数据模型/契约/状态机/复用点与待决策清单见 `docs/modules/channel-预研.md`，
> 技术决策见 `docs/adr/ADR-001-channel-data-model.md`（提议）。结论：**无新增内核能力需求**，
> 唯一硬前置为 #13（权限点与后台账号模型）；确认当日起可并行开发 §1/§2。

## 1. channel：运营商移动端身份（小程序）

- [ ] 1.1 迁移：`chn_operator`、`chn_guarantee`、`chn_commission`、`chn_settlement`、`chn_annual_target`、`chn_attribution`（详细设计 4.2.18 附）
- [ ] 1.2 红线进 schema：`chn_operator.referrer_operator_id` 单列、无上级字段、不能自建下级；SQL 层与 contract 均无多级结构
- [ ] 1.3 运营商身份卡（个人中心显示身份与所属区域）；区域推广专属链接/邀请码
- [ ] 1.4 归属关系记录（`chn_attribution`：用户注册来源绑定运营商）
- [ ] 1.5 佣金明细、提现申请、业绩概览（本月新增付费会员/佣金合计/保底任务完成度）
- [ ] 1.6 状态机 #16 运营商（申请中→已开通→已停用，停用须重新申请）+ 表驱动测试

## 2. channel：佣金与结算（后台）

- [ ] 2.1 开户审核：区域查重一地一家；代理费首年 3 万/次年 2 万收款登记（入 settings 可配）；保证金 1 万可退台账（`chn_guarantee` 预收款，不与收入混记）
- [ ] 2.2 佣金规则配置（首单 30%/续费 10% 初始值，SettingsSchema 全后台可配；计佣范围按 #16 结论）
- [ ] 2.3 订阅 `OrderPaid`、`MemberActivated`：按配置比例生成 `chn_commission` 流水并写入 ratio_snapshot（配置变更不影响历史流水）
- [ ] 2.4 月结结算单状态机 #17（待结算→已结算→已打款，打款不可撤销）：生成→财务复核→对公打款→留凭证；**复核与执行不得同一人**（内核策略强制互斥，违反 denied 记审计）
- [ ] 2.5 保底任务跟踪：12 个月 ≥50 付费会员或 ≥3 落地项目；进度与预警（`chn_annual_target`）
- [ ] 2.6 归属纠错审批留痕（`chn_attribution.correct_approval`）
- [ ] 2.7 对账导出（按运营商/区域/时间）走导出中心
- [ ] 2.8 事件：`CommissionAccrued`、`SettlementPaid`、`OperatorStatusChanged` Schema 与发布
- [ ] 2.9 contract：`OperatorOf`、`CommissionOf`、`SettlementApprove/Pay`

## 3. 后台 RBAC 与权限点

- [ ] 3.1 `sys_admin_user/sys_admin_role` 迁移；五内置角色（超管/运营/审核员/财务/渠道经理）种子；角色带 space_id
- [ ] 3.2 AdminSection 增强：菜单级（路由可见性）+ 操作级（action 级按钮可用性）两级；权限点命名 `<module>:<resource>:<action>`，统一登记在 contracts
- [ ] 3.3 服务端强制校验为唯一权威（前端隐藏仅体验）；后台登录 denied 记审计（防爆破）
- [ ] 3.4 后台账号强制二次验证；敏感操作（退款/封禁/改价格比例/导出/结算）二次确认 + 审计
- [ ] 3.5 内核 policy「复核≠执行」互斥规则（同一结算单/退款单操作人≠复核人）
- [ ] 3.6 五角色-权限点矩阵表驱动用例（含「明确禁止」列的负向用例：运营不得碰资金、审核员不得处置内容以外的资源等，详细设计 10.3）
- [ ] 3.7 admin-web shell 按 RBAC 过滤菜单

## 4. 治理分区 governance/

- [ ] 4.1 用户 360 详情：聚合 identity（UserBrief/状态）、profile（名片）、verify（认证/核验/单位关系）、membership（会员/子账号）、content（发布列表）、trust（违规记录）、trade（订单）只读接口；Tab 分区展示，每区带模块来源标注
- [ ] 4.2 用户检索与筛选：手机号（盲索引精确）、昵称、认证状态、会员等级、注册时间、状态组合筛选；导出走审计
- [ ] 4.3 封禁与限制：封禁/解禁/限制发布（trust contract），二次确认 + 审计
- [ ] 4.4 运营代操作清单 10 项落地（入口/权限点/审计级别，详细设计 10.2）：代处理超期核验、重置组织管理员、转移企业版主账号、强制解除子账号、履约手动补偿、猫立方/圈子封禁解散、归属纠错审批、会员人工调整（双人复核）、定向公告发布、死信重放/任务重跑
- [ ] 4.5 代操作全部二次确认 + 全量审计（detail 含操作前后值快照）

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
- [ ] 6.3 状态机 #3 认证申请（待审核→已通过/已驳回）、#4 身份核验 + 表驱动测试；驳回必须给原因、可修改重提
- [x] 6.4 身份核验：`GET /api/mp/v1/affiliations/mine`（含待核验状态驱动置灰/红框）、`POST /affiliations/{id}/verify`（组织管理员，**先到先处理**：重复处理返回 404/100006）、`POST /api/admin/v1/affiliations/{id}/resolve` 平台代处理（超期 `OrphanAffiliations` 查询）、`POST /affiliations/{id}/primary` 唯一主单位；事件 `verify.AffiliationVerified.v1`
- [x] 6.5 组织管理员：企业/机构认证通过时申请人自动成为首任管理员（`UpsertActiveOrgAdmin`）；邀请需对方确认（`POST /orgs/{id}/admins` → `POST /org-admin-invites/{id}/confirm`）；平台重置/移除 `POST /api/admin/v1/orgs/{id}/admins/reset`；事件 `verify.OrgAdminChanged.v1`
- [x] 6.6 一人多单位：`vfy_affiliation` 一对多（user+org 唯一）；状态驱动置灰（0 待核验）/红框（1 已核验）；`is_primary` 唯一主单位由 `SetPrimary` 维护
- [ ] 6.7 Badge 统一机制：`vfy_badge_def`/`vfy_badge_grant` 输出 {code,label,level}（含「猫平方官方合作认证」配置项）；新增标识类型=加配置+前端样式，不改接口
- [x] 6.8 事件与不可改：认证材料不可编辑（重复提交为新的待审记录）；`verify.AffiliationVerified.v1` / `verify.OrgAdminChanged.v1` 事件 Schema 已入契约并生成（`contracts/events/`），notice 模板同步
- [ ] 6.9 小程序认证页：猫²认证列表（两大类五项状态）、实名页、材料上传页、身份核验列表（组织视角）、个人企业身份认证页（详细设计 5.11）

## 7. 统计看板与导出中心

- [ ] 7.1 12 项核心指标口径落地（详细设计 12.2）：从业务表实时/定时聚合，禁止为统计反向污染业务表；每个指标注明口径与数据来源（hover 显示）
- [ ] 7.2 小程序埋点：8 个漏斗事件（register/profile_complete/feed_click/search/detail_view/contact_initiate/member_quote/order_paid），事件名与属性在 contracts/telemetry 登记；独立 `tlm_` 前缀存储
- [ ] 7.3 6 张看板：平台总览（L1+L2 漏斗）、圈子/猫立方、渠道（渠道经理仅见本人区域）、资金、安全、运营商移动端业绩概览
- [ ] 7.4 导出中心：申请（模板+时间范围）→ 异步生成落 OSS → 下载带水印 + 导出审计（action=export）；对账导出、结算明细、用户列表、审核记录四个模板；导出内容遵循敏感字段白名单（不含明文联系方式）

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
