# p3-collaboration · 协作

## Why

阶段 2 已跑通「找对人 + 联系上 + 付费开通」主链路（《技术方案 v2.5》第 13 章）。阶段 3 按依赖阶梯向上交付 R3 的 `cube`、R4 的 `circle` 与 R6 的 `notice`，把平台从「认识」推进到「办成」——这是 PRD 1.4 中猫立方协作工作台与圈子拉新漏斗（「在圈子里认识 → 到猫立方里办成」，详细设计 5.8/5.9）的核心价值闭环。本阶段同时补完 `trade` 的分摊能力（成团强一致、开通最终一致，《技术方案 v2.5》8.4 / 详细设计 7.5，支付合规红线在 p0 已由待确认 #1 闭环）、`msg` 的群聊能力（`SessionMemberResolver` 由 cube 注册，msg 不认识猫立方，《技术方案 v2.5》8.6），并以数据驱动的 `NoticeTemplate` 落地 PRD 11.3 共 36 条触达（详细设计 8.3）。分摊成团是本阶段最重的强一致场景，退出标准因此锁定支付幂等、并发成团与状态机测试。

## What Changes

- **cube 全量**：猫立方空间创建与「关于我们」（独立于组织实体，可关联一个组织复用字段）；邀请制成员体系（猫王必配 + 猫蜜必配；微信卡片/二维码/邀请码 7 天有效可重发；需对方确认、可配免确认；已实名门槛；被移除不可自行重进；猫王可转让）；研究模块（项目 8 字段、分工待办、研究记录 → 决策结论、可关闭灰字可重开）；承接模块（项目 7 字段、任务分配、重点客户报备、项目进度跟踪）；任务四态（待办 → 进行中 → 待确认 → 已完成，驳回须填原因退回进行中，逾期只提醒不自动通过，已完成可重开）；内部工作安排（任务可不挂项目）；订阅三档（基础免费 1 个 ≤10 人 / 进阶 3 个 ≤50 人 ¥999 年 / 高阶不限 ¥2999 年，全部 `SettingsSchema` 后台可配）；分摊（猫王设人数 N 不设价，2≤N≤档上限，每人=档价÷N，默认 7 天截止，满 N 即时成团，超额达 N 关闭入口，截止前可退出退款，未成团逐笔幂等全额原路退回且全部完成才终结，成团成员自动成为成员不受 10 人限、标「共建成员」，续费可重设名单）；立方内项目 = 引用 + 本地状态（`cub_project.content_project_id` 可空无外键，同一平台项目可进多立方状态独立，平台下架显示「平台已下架」不删除，引入规则落 `cub_project_link_req`）；状态机 #5~#9、#13；注册 `OrderFulfiller(biz_type=cube_plan)`、`SessionMemberResolver`、`CapabilityProvider`、`ReviewSubjectProvider`、`EntityResolver`
- **circle 全量**：常设/临时圈（7/30/90/180 天建圈选定不可改，到期前 7 天提醒，到期转「已结束」只读不删不可续期可转常设，圈主解散二次确认 + 成员数提示、永久删除并通知全员，新圈 7 天内仅可搜索不进推荐流）；加入四路（广场申请圈主审 / 邀请链接=圈剩余存续期可重置旧链作废 / 组织管理员直接加入 / 活动会议报名自动入圈）；圈内发布（个人需求标题 ≤30 字、内容 ≤500 字、有效期默认 30 天到期沉底可一键续期 30 天、VIP 可全平台标注来源圈、联系方式会员可见；圈内动态图文）；限额（每人每日跨圈合计 3 条与会员无关、圈主公告每圈每日 ≤3 条不占个人额度、建圈 ≤1/日同时最多拥有 3 个、单圈挂载 ≤50 条后台可配，全部 `QuotaDef` 落地）；挂载仅我发布的项目/产品引用不复制、他人内容申请挂载需对方确认；「到猫立方里办」四前提校验后调 `cube.CreateProjectFromRef` 只带标题正文来源引用，来源圈条目显示「已转入猫立方办」状态独立；圈内不聊天不评论不点赞；圈内动态/挂载可分享微信落地页显示「来自 XX 圈」+ 加入入口；状态机 #10、#11
- **notice 通知中心**：`NoticeTemplate` 数据驱动（事件 → 接收人规则 → 文案 → 渠道站内/订阅/短信 → 是否合并），PRD 11.3 共 36 条触达逐条映射为模板注册；业务模块从不直接发通知；订阅授权额度 `ntc_subscribe_quota` 上下文动作处申请、发送时消耗、无额度降级站内未读；同圈同日合并 `ntc_digest` upsert + 日结任务；站内信兜底任何事件至少一条；资金与审核类不可关闭
- **msg 群聊**：cube 注册 `SessionMemberResolver`，群聊名称=立方名·模块名/项目名，消息异步检测走 trust（`MessageSent`）
- **trade 分摊完成**：`trd_apportion_group`/`trd_pledge`，成团在 trade 事务内锁行计数强一致，到期扫描逐笔幂等退款，发布 `ApportionFormed/ApportionFailed`
- **小程序**：猫立方 Tab 全部页面（空态/创建/加入邀请码/详情四模块/任务/报备/成员/订阅分摊 15 页）、圈子 Tab（我的圈子进行中/已结束分区、创建、主题圈详情四区块、非成员仅见关于本圈 + 申请加入）、消息 Tab 群聊会话
- **bff**：猫立方首页聚合（立方列表 + 待办 + 未读单次返回）

## Impact

- **capabilities**：cube、circle、notice、msg、trade、miniapp
- **后端**：`apps/server/internal/modules/cube/`、`circle/`、`notice/`（新建，含 `contract/`、`module.go`、`admin/`、`migrations/`、`internal/{transport,app,domain,infra}`）；`apps/server/internal/modules/trade/`（分摊子域补全）；`apps/server/internal/modules/msg/`（群聊会话与 `SessionMemberResolver` 开口实现）；`apps/server/internal/bff/miniapp/`（猫立方首页聚合）；`apps/server/internal/app/wire.go`（装配新模块）
- **数据**：新表前缀 `cub_`（9 张）、`cir_`（6 张）、`ntc_`（4 张）；`trd_apportion_group`/`trd_pledge` 随 trade 迁移落地
- **契约**：`contracts/openapi/mp.yaml` 新增 cube/circle/notice/msg 群组路径；`contracts/events/` 新增 cube、circle 事件 Schema 与 trade 分摊事件；`contracts/errors/` 登记三个新模块错误码段
- **前端**：`apps/miniapp/src/features/cube/`、`features/circle/`（分包）、`features/msg/`（群聊）、`shell/` 三个 Tab 接入
- **依赖前置**：p1 的 kernel（状态机/quota/scheduler/policy）、p2 的 identity（已实名能力）、verify、content（`ContentKind`/`EntityResolver`）、trade（订单/支付）、membership（VIP 可见范围判定）、msg（单聊骨架）均已就绪；本阶段不改这些模块的既有行为
- **不做**：后台管理界面（`AdminSection` 仅注册菜单与配置项声明，页面在 p4）；渠道佣金（p4）；推荐流中的圈卡片混排与 `CircleCreated` 搜索投影消费属 discovery，本阶段只保证事件发出（待确认 #22 已在 p0 闭环）
