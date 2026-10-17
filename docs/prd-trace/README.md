# PRD 11.4 验收追溯矩阵（p6 §7.1/§7.2）

- 来源：`docs/猫平方-产品需求说明书-v2.5（对外交付版）.docx` §11.4「验收标准」（逐条抽取）。
- 状态：`✅` 已落地并有测试证据；`◐` 部分落地/口径待确认；`❌` 未落地；`—` 属外部动作（非代码）。
- 2026-10-17 复核：p4（运营商/渠道/RBAC）已全部完成，原 p4 依赖项转为 ✅；剩余 ◐ 均为产品口径或发版动作。
- 证据列给出代码位置或集成测试名（`apps/server/internal/app/integration_test.go` 等）。
- 阶段性完成度与阻塞汇总见 `docs/prd-trace/stage-delivery-report.md`。

## 账号与入驻

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 微信授权登录成功 | ✅ | `modules/identity`（`/api/mp/v1/auth/wechat`）；`TestIntegrationLogin*` |
| 未填手机号不能进入主流程 | ✅ | identity spec（p1 §2）；`pages/auth` 引导补手机号（三动线） |
| 入驻表单必填校验与 8.1 一致，提交可见认证状态 | ✅ | `modules/profile` 表单校验；`pages/card` / `pages/verify` |
| 未实名用户点发布/建圈/建猫立方被拦截并引导 | ✅ | `TestIntegrationPermissionMatrixFull`（建立方 130003、建圈 403、内容提交 110004） |

## 首页与搜索

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 首页卡片流按类型筛选、六类字段与 5.2.3 一致 | ✅ | `bff` `/feed`、`modules/content` 四类 + discovery；`pages/square` |
| 「换一批」刷出不同内容、空态非白屏 | ✅ | `POST /api/mp/v1/feed/refresh`；`StateView` 空/载/错态 |
| 搜索命中六类（人/企业/项目/产品/活动/会议） | ✅ | `modules/discovery` `/search` + `dsc_search_doc`；`TestIntegrationSearch` |

## 详情页与联系

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 未登录/非会员「联系Ta」引导开通会员；会员可见手机号并发起单聊 | ✅ | `bff` `/aggregate/contact`（120007）、`msg` `/sessions`（70004）；`TestIntegrationPermissionMatrixFull` |
| 详情页字段完整无空白 | ✅ | `bff` `/aggregate/detail` 按角色裁剪 + `member_gate/unlock_reason` |

## 消息

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 系统通知/单聊/群聊三类会话分列 | ✅ | `modules/msg` `Sessions`（session type）；`pages/messages` |
| 未读数准确、进入会话清零 | ✅ | `TestIntegrationMsgUnread`；`POST /sessions/{id}/read` |
| 一期不出现实时推送报错；轮询间隔符合 5.5.3 | ✅ | `RealtimeNoop` + 小程序 `pages/msg/chat` 轮询 |

## 圈子

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 常设/临时均可建，临时须选时长 | ✅ | `modules/circle`；`TestIntegrationCircle*` |
| 每人每日 ≤1、同时 ≤3 限额；加入不受限 | ✅ | circle 配额门禁；`TestIntegrationP3ExitCriteria`/配额用例 |
| 广场可发现、申请或邀请加入 | ✅ | `/api/mp/v1/circles` 广场、join 四路 |
| 圈内发布跨圈合计 3 条/日；圈主公告每圈 ≤3/日 不计个人额度 | ✅ | circle 发布配额与公告规则 |
| 需求到期自动沉底并标「已过期」、可一键续期 | ✅ | `kernel/expiry` + circle 需求；`TestIntegrationExpirySample` |
| 临时圈到期结束只读不删不可续、到期前可转常设、解散二次确认永久删 | ✅ | `pages/circle/settings`（转常设/解散二次确认） |
| 「到猫立方里办」四前提校验，不满足置灰给引导 | ✅ | circle 挂载 `mount` 前提校验 |

## 猫立方

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 建立方自动为猫王、必须指定 1 名猫蜜 | ✅ | `modules/cube`；`TestIntegrationCube*` |
| 邀请制（不可搜索加入）、邀请有效期 7 天 | ✅ | `cub_invite`（expires_at） |
| 已实名用户可进任意档位、不校验会员等级 | ✅ | cube `requireRealname`（非会员门槛） |
| 四模块入口；项目「已关闭」灰字可重开 | ✅ | `pages/cube/project`、`CloseProject/Reopen` |
| 任务四态流转；负责人提交→待确认，确认人确认才完成，逾期不自动通过 | ✅ | cube 任务状态机 #8 |
| 报备提交后企业收通知、确认计入进度、驳回可改重提 | ✅ | `cub_report` + notice |
| 分摊：达标自动开通；未达标到期全额原路退回 | ✅ | `trd_apportion_group/trd_pledge` + 幂等退款；`TestIntegrationP3ExitCriteria` |
| 立方内项目状态与平台状态互不影响 | ✅ | `cub_project.status` vs `content.platform_status` |

## 认证与核验

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 各类认证必填材料与 8.4 一致、驳回可重提 | ✅ | `verify` 五类（1 实名实时二要素；2~5 材料 ≤3，平台/企业审）；状态机 #3/#4；`TestIntegrationVerifyRealname`、`TestIntegrationVerifyCertTypesAndOrgAdmin`、`TestIntegrationVerifyStateMachines` |
| 组织管理员审核成员核验；超 7 天平台代处理 | ✅ | 组织管理员机制（首任=企业认证申请人、邀请需确认、平台重置）+ 一人多单位核验（先到先处理）+ `POST /api/admin/v1/affiliations/{id}/resolve` 平台代处理；小程序「待核验（红点）」；`pages/verify` |
| 认证标识统一机制（新增类型不改接口） | ✅ | `vfy_badge_def/grant` + 后台 `GET/PUT /api/admin/v1/badges`、`POST /badges/grant`；admin-web「认证标识」 |

## AI 一期（p5，受开关门禁）

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 一句话入驻/发布草稿/自然语言搜索 | ✅ | p5 27/27：`ai` 模块 + `pages/card|content|search|support`；产品 #3 已确认纳入一期；生产经独立 AI 网关（ADR-003），开关默认关、放量为配置动作 |
| AI 不可用关闭开关无影响（原表单全链路） | ✅ | `TestIntegrationAIFlagGateAndDegrade` |
| 采纳率与成本可观测 | ✅ | `catcube_ai_adopted_total`/`catcube_ai_cost_fen_total`；`ai_call_log` 留痕 |
| 隐私红线（手机号/证件号不进提示词） | ✅ | `sanitizeForPrompt` 单测 + 集成测试 |

## 发布与管理

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 项目/产品/会议/活动表单字段各自独立 | ✅ | `modules/content` kinds（字段池 + TagSet）；`TestIntegrationContentFlow` |
| 发布进审核流程、结果通知作者 | ✅ | `trust.Review` + `notice` 模板；`TestIntegrationModerationTimeoutGate` |
| 图片上传符合 8.9.2 格式与大小限制 | ✅ | `shared/media`（jpeg/png、≤5MB）；`TestIntegrationMedia*` |

## 会员与支付

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 价格与档位按后台配置、不硬编码；多年套餐折扣正确 | ✅ | `membership.Settings`（`sys_settings`）+ `price_snapshot(price_fen,years)` |
| 支付成功权益即时生效；未支付订单超时关闭 | ✅ | `trade` 履约 + `trade.close_expired`；`TestIntegrationTradeFlow` |
| 支付成功率/回调审计 | ✅ | `catcube_order_paid_total`/`catcube_pay_notify_failed_total`；`TestIntegrationPayReplayAndForgery` |

## 运营商（p4 范围）

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 运营商身份移动端可见、推广链接/二维码与归属 | ✅ | `GET /api/mp/v1/channel/operator`、`/channel/link`、`/channel/attribution`；小程序 `pages/channel/index`；`TestIntegrationChannelOperator`/`TestIntegrationChannelFeesAttribution` |
| 佣金明细/提现/业绩概览准确 | ✅ | `/channel/overview`、`/channel/commissions`、`/channel/withdraw`；`TestIntegrationChannelCommissionSettlement` |
| 归属关系纠错留审批痕迹 | ✅ | `GET /channel/attributions` + `POST /channel/attributions/{id}/correct`（原因+前后值）+ 审计 `channel:attribution:correct`；`TestIntegrationChannelFeesAttribution` |

## 总管理后台

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 渠道管理七模块齐备、佣金比例/结算周期可配 | ✅ | 开户/停用、费用与保证金台账、佣金规则（SettingsSchema 热生效）、归属纠错、结算生成/复核/打款/驳回、业绩目标；admin-web「运营商」「渠道结算」「配置中心」；`TestIntegrationChannel*` + `TestIntegrationAdminRBAC` |
| 认证/归属/标识/导出/审计/看板后台能力 | ✅ | 五类认证审核与组织机制、标识配置、导出中心（对账/订单/用户/审核，水印+审计）、审计轨迹五维查询、看板四分组与漏斗 |
| 猫立方与圈子的内容处置、封禁、批量操作 | ◐ | 内容下架、立方封禁/解散、圈子解散 ✅（admin-web 页 + 审计）；**批量操作未提供**（逐条处置），待产品确认是否需要 |
| 数据概览口径与 6.5 一致 | ✅ | `GET /api/admin/v1/dashboard`（四分组）+ `/dashboard/funnel`；admin-web 概览页已对接；口径见 `docs/ops/dashboards.md` |

## 跨端一致性

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 小程序与后台数据一致 | ✅ | 同一后端 API（admin 与 mp 同源），无独立数据副本 |
| 金额单位元、配置改动即时生效 | ✅ | 前台 fen→元 展示；`sys_settings`/`sys_feature_flag` 热生效 |

## 会员与企业版

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 企业版仅企业认证账号可开通；子账号 ≤3 共享权益、移除后可再建、不可单独开通 | ✅ | `membership` 企业认证门禁 + `mbr_sub_account`（唯一活跃约束） |
| 年费退款按 8.3.1：系统原因重复支付自动退回；不符合规则不受理并说明 | ✅ | 退款幂等 + `RefundQuote`；`TestIntegrationTradeFlow` |
| 企业版到期未续费：主/子账号同步回落、子账号归属保留 30 天，期内续费恢复 | ✅ | `membership.ExpireDue`：主账号到期 → 子账号同步回落 + 归属保留 30 天后清理（`PurgeRemovedSubs`）；`TestIntegrationEnterpriseSubExpiry` |

## 圈子触达（notice）

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 圈内新增需求/动态通知其他成员（不含发布人）、同圈同日合并、可关闭该分类 | ✅ | `notice` 合并 + 偏好；`TestIntegrationNotice*` |
| 圈主解散后全员收到（不可关闭）、成员侧移除该圈 | ✅ | 资金/审核类不可关闭规则；circle 解散通知 |
| 临时圈到期/圈主注销致圈结束，全员收到通知 | ✅ | `kernel/expiry` + notice 模板 |

## 圈子邀请与拉人

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 邀请链接有效期随圈、结束/解散失效、重置后旧链接作废 | ✅ | `cir_invite`（随圈状态 + 重置） |
| 圈主与成员均可生成邀请链接/二维码/圈名片 | ✅ | circle 邀请接口；`pages/circle/detail` 邀请/分享 |
| 邀请直接加入；广场申请需圈主审核 | ✅ | join 四路 |
| 非成员（含未登录）仅见「关于本圈」并可申请 | ✅ | `bff` `/aggregate/circle-detail` 裁剪视图 |
| 圈名片图片含圈名/简介/成员数/圈主/二维码 | ◐ | 分享能力就绪；**名片图片渲染口径待产品确认** |
| 圈内动态/挂载分享微信，落地页「来自 XX 圈」+加入入口 | ◐ | 小程序分享路径已接；**落地页归小程序发版** |

## 首页与画像

| 验收点 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 「完善资料」提示条按身份类型跳对应名片页（不跳实名页） | ✅ | `pages/mine` + identity_type 分支 |
| 第二层匹配画像填全后提示条消失 | ✅ | profile 完成度驱动 |
| 组织名片展示组织字段、不出现个人字段 | ✅ | `modules/profile` 按 identity_type 渲染；`pages/card` |

## 结论

- 主链路（p2）、协作（p3）、运营商与渠道后台（p4）、AI 文本场景（p5）、硬化（p6 已做项）验收点均已落地并有测试证据。
- **p4 阻塞已解除**（2026-10-17）：运营商/渠道/RBAC 全部完成，原 ❌ 项转为 ✅。
- 剩余 ◐ 3 项均为非 p4 事项：批量处置范围（产品确认）、圈名片图片口径（产品）、分享落地页（小程序发版）。
- 部分落地项已标注 ◐ 并给出待确认口径，明细见 `pending-confirmations.md`。
