# p2-main-loop · 主链路

## Why

PRD 1.4.1 把「找对人 + 联系上」定为一期主场景，「联系上」是 VIP 的第一权益、唯一能立刻验证付费意愿的动作，因此会员支付必须在主链路阶段跑通而不是推到协作之后（《技术方案 v2.5》第 13 章的调整）。本阶段交付从「注册入驻 → 画像 → 广场匹配/搜索 → 详情 → 关注/联系Ta → 开通会员 → 单聊」的完整闭环，同时把 trust 的内容安全链路补完（发布即过审，PRD 9.3 是过审与合规底线）。

## What Changes

- **profile**：个人/组织名片（一账号一张，注册时身份类型决定）、三层画像、画像完成度五项判定（驱动首页「完善资料」提示条）、对外展示设置、名片分享、组织资质证明、认证主体信息锁定；生产 `ProfileUpdated`（含 MatchProfile/SearchDoc）
- **verify（仅实名）**：第三方二要素实时核验、免证件照上传、协议勾选、状态机 #1
- **content**：`ContentKind` 插件机制 + 项目/产品/会议/活动四类（信封表 `cnt_item` + 四张类型子表）、状态机 #2（审核中→展示中→已下架）、会员可见字段清单（核心优势、合作联系）、发布门槛=已实名、我的发布、详情
- **social**：关注/粉丝/收藏（目标统一 EntityRef）、关注=订阅（被关注者发新内容推系统通知）、`soc_follow_unread`
- **discovery（规则版）**：事件投影 `dsc_search_doc`/`dsc_match_candidates`；「先给后要」注册即给 3 条；评分=需求供给互补 40%+行业 25%+区域 20%+身份互补 15%（权重配置化带版本）；同分按会员等级（企业版>SVIP>VIP>普通，不设推荐位）；推荐理由规则模板；PG 搜索（pg_trgm）；标签筛选与关键词检索独立可用
- **trust（完整链路）**：敏感词前置 → 文本同步机检 → 图片异步机检（回调前审核中占位）→ 举报 → 24h 人工复核 → 属实下架+记分（3 次限发 7 天）→ 严重者封号；ReviewSubjectProvider 由 content/profile 注册
- **trade**：`trd_order/payment/refund/recon`、微信支付回调验签+幂等、Idempotency-Key、每日对账、`OrderFulfiller` 开口
- **membership**：VIP 年费锚点 × 年限 × 折扣系数（3 年 0.9、5 年 0.8，全后台可配）、PriceQuote 同时展示原价与折后价、OrderPaid→履约开通/续期、退款金额=剩余完整年数×年费×实付折扣系数（取订单快照）、企业版+3 子账号（手机号邀请、须已注册已实名）、到期回落（子账号归属保留 30 天）、状态机 #12、`MemberActivated`→identity `authz_ver` 递增 + discovery 权重刷新
- **msg（异步单聊）**：详情页「联系Ta」发起（会员权益）、`(min_uid,max_uid)` 唯一会话、未读=会话最大序号−已读游标、cursor 增量同步、前台 3 秒轮询 + 无新消息指数退避
- **bff/miniapp**：首页 feed 单次聚合（卡片流+画像进度+未读数）、详情页聚合（名片+认证标识+会员等级+关注状态）
- **小程序主链路页面**：登录注册一体化、广场 Tab（六类卡片流+换一批+类型筛选+完善资料提示条三条负向声明）、搜索、六类详情页、四类发布表单、名片编辑（个人/组织）、会员开通页（支付前明示「开通后不支持退款」）、消息列表与单聊会话；共享组件 ContentCard/PolicyGate/TaxonomyPicker/Uploader/CursorList/EmptyState/SubscribeAsk/PayButton
- **各模块 admin/**：用户列表、内容管理、审核队列、订单、会员列表的基础管理接口与菜单声明

## Impact

- **capabilities**：profile、verify、content、social、discovery、trust、trade、membership、msg、miniapp、admin-web
- **后端**：`modules/{profile,verify,content,social,discovery,trade,membership,msg}/` 新建；`modules/trust/` 补全举报/记分/复核；`bff/miniapp/` 新建（feed、详情聚合）；`app/wire.go` 装配
- **数据**：新表前缀 `prf_/vfy_/cnt_/soc_/dsc_/trd_/mbr_/msg_`；`trs_` 补 `trs_report/trs_violation_score/trs_restriction`
- **契约**：`contracts/openapi/mp.yaml` 主链路全部路径（详细设计 6.2）；`contracts/events/` 新增 profile/verify/content/trust/trade/membership/social/msg 事件 Schema（附录 C）；错误码段登记
- **前端**：`apps/miniapp/src/shell/`（5 Tab）、`features/{discovery,content,profile,verify,membership,trade,msg,social}/`、`shared/` 组件库；`apps/admin-web/src/features/` 基础页面
- **依赖前置**：p1 的 kernel 全部机制、identity 登录、trust 骨架、dict、media 已就绪
- **不做**：cube/circle/notice/channel（p3、p4）；verify 的个人/企业/机构认证与身份核验（p4）；AI（p5）
