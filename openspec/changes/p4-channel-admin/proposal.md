# p4-channel-admin · 渠道与后台

## Why

主链路与协作闭环已通，平台进入「可运营」阶段。本阶段交付三件互相咬合的事：① `channel` 渠道体系（运营商是 PRD 7.x 的增长引擎，但佣金、结算、保证金全部是资金合规敏感区）；② 后台全功能与 RBAC（五角色 + 操作级权限点 + 「复核≠执行」机制化，详细设计 10.3——**待确认 #13 是立项阻塞项，未确认本变更不开工**）；③ 认证全类型（verify 补齐个人/企业/机构认证与身份核验，详细设计 3.11）。对账结算闭环是本阶段退出标准。

## What Changes

- **channel**：运营商移动端身份（身份卡、区域推广链接/邀请码、归属关系、佣金明细、提现申请、业绩概览）；总后台渠道管理（开户审核区域查重一地一家、代理费收款登记、保证金台账预收款不与收入混记、佣金规则首单 30%/续费 10% 后台可配、月结结算单状态机 #17、保底任务 12 个月 ≥50 付费会员或 ≥3 落地项目、归属纠错审批留痕）；红线进 schema：`referrer_operator_id` 单列无上级字段；状态机 #16
- **后台 RBAC**：固定五角色（超管/运营/审核员/财务/渠道经理）；AdminSection 增强为菜单级 + 操作级两级权限点（`<module>:<resource>:<action>` 统一登记，服务端强制校验为唯一权威）；后台账号强制二次验证；敏感操作（退款/封禁/改价/导出/结算）二次确认 + 审计；「复核与执行不得同一人」由内核策略强制互斥；角色带 space_id
- **治理分区 governance/**：用户 360 详情（聚合 identity/profile/verify/membership/content/trust/trade 只读接口，Tab 分区带模块来源标注）、用户检索筛选（手机号盲索引精确）、封禁与限制；运营代操作清单 10 项全部二次确认 + 全量审计（含前后值快照）
- **审计体系落地**：AuditPoint 注册表表驱动 + CI 校验（敏感动词 refund/ban/export/config/transfer/proxy 未声明即失败）；17 模块审计点清单逐一落地；后台审计页五维筛选 + target_ref 反查 + 快照对比；安全告警（denied 突增/登录锁定激增/联系方式查看超阈/审计断链）
- **verify 认证全类型**：个人认证（身份证或专业资质任选，平台审 1-3 工作日）、个人企业身份认证（选已入驻企业+职务+任职证明，企业审）、企业认证（营业执照+法人身份证，平台审）、机构认证（证件/批文+负责人身份证明+授权书，平台审）、身份核验（组织管理员审，先到先处理，超 7 天平台可代处理）、组织管理员机制（首任=入驻申请人、添加需对方确认、失联可申请平台重置）、一人多单位 affiliation、Badge 统一输出 {code,label,level}（含「猫平方官方合作认证」）、未核验置灰/核验通过红框；状态机 #3、#4
- **统计看板与导出中心**：指标体系 L1~L4 与 12 项核心指标口径落地；小程序埋点仅漏斗必需 8 事件（独立 `tlm_` 前缀）；6 张看板；导出走导出中心（申请→异步生成落 OSS→下载带水印+导出审计）
- **admin-web 全部 features 页面完成**

## Impact

- **capabilities**：channel、admin-web、verify、platform-kernel（审计/RBAC 机制增强）、miniapp（运营商身份页）
- **后端**：`modules/channel/` 新建；`modules/verify/` 补全；各模块 `admin/` 补全操作级权限点；`bff/admin/` 治理聚合；kernel/policy 增加「复核≠执行」互斥规则支持
- **数据**：新表前缀 `chn_`（6 张）；`sys_admin_user/sys_admin_role`；`vfy_affiliation/vfy_org_admin` 等补齐；`tlm_` 埋点表
- **契约**：`contracts/openapi/admin.yaml` 全量；运营商小程序路径；`channel` 事件 Schema（`CommissionAccrued/SettlementPaid/OperatorStatusChanged`）
- **前端**：`apps/admin-web/src/features/` 全部模块页面 + `governance/` 分区；`apps/miniapp` 运营商身份页（68~73 页）
- **依赖前置**：p1~p3 全部；p0 待确认 #13（五角色）、#16（佣金口径）已闭环
- **不做**：AI（p5）；S1 部署与压测（p6）
