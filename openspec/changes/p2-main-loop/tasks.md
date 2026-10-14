# p2-main-loop · 任务清单

## 1. profile（R1）

- [ ] 1.1 迁移：`prf_card`（详细设计 4.2.8 全字段：subject_type_code/三层画像字段/contact_phone_enc/visibility_settings/profile_tier/search_doc_version/owner_space_id）
- [ ] 1.2 名片 CRUD：`GET/PUT /cards/me`、`GET /cards/{ref}`（他人名片按对外展示设置过滤）、`POST /cards/me/share`
- [ ] 1.3 画像完成度 `GET /cards/me/completion`：第二层五项（区域/行业/合作需求/合作提供/简介）任一为空即未完成；与实名无关
- [ ] 1.4 组织名片资质证明多图上传（接 media.Bind）
- [ ] 1.5 认证主体信息锁定：订阅 `CertificationApproved` 后对应字段禁自改
- [ ] 1.6 事件生产：`ProfileUpdated`/`OrgCardUpdated`（含 MatchProfile/SearchDoc，不含敏感字段，带 search_doc_version）
- [ ] 1.7 注册扩展点：`ReviewSubjectProvider`（名片文本/头像）、`EntityResolver`（user/org）
- [ ] 1.8 domain 单元测试：完成度判定、展示设置过滤、认证锁定

## 2. verify（R2，仅实名）

- [ ] 2.1 迁移：`vfy_certification`（4.2.11）、`vfy_badge_def`、`vfy_badge_grant`
- [ ] 2.2 实名认证：真实姓名+身份证号+协议勾选；第三方二要素接口实时核验，不上传证件照；人脸核身高风险触发（接口预留）
- [ ] 2.3 状态机 #1 实名（未实名→核验中→已实名/核验失败）StateMachineDef + 表驱动测试
- [ ] 2.4 核验通过回写 `idn_user.realname_status`（经 identity contract，不读他人表）并授予「实名」Badge
- [ ] 2.5 注册 CapabilityProvider（已实名/已认证/组织管理员——后两者本阶段恒否）；`ReviewSubjectProvider`（认证材料）开口预留
- [ ] 2.6 contract：`Certify`、`BadgesOf(ref)`；事件 `CertificationApproved/Rejected` Schema

## 3. content（R2）

- [ ] 3.1 迁移：`cnt_item` 信封表（4.2.13）+ `cnt_project/cnt_product/cnt_meeting/cnt_event` 四张类型子表（4.2.14，类型化列筛选排序、展示用少量扩展走 JSONB）
- [ ] 3.2 `ContentKind` 注册表：字段 schema 与校验、卡片字段（PRD 5.2.3）、详情字段、会员可见字段清单、审核文本/图片提取、SearchDoc 映射；四类 Kind 各自注册
- [ ] 3.3 四类发布表单接口：`POST /contents`（kind 区分）、`POST /contents/{id}/submit`、`GET /contents/{id}`、`GET /contents/mine?kind=`、`POST /contents/{id}/close`
- [ ] 3.4 发布门槛策略：已实名（CapabilityProvider）；未实名拦截记审计（高频告警）
- [ ] 3.5 状态机 #2（审核中→展示中→已下架；已下架不可自行恢复、可申诉）+ 表驱动测试（合法/非法转移）
- [ ] 3.6 订阅 `ModerationPassed/Rejected`、`ReportConfirmed` 驱动状态机（trust 不直接改 cnt_item.status）
- [ ] 3.7 会员可见字段：`Get` 输出按 policy Decision 过滤核心优势/合作联系
- [ ] 3.8 事件生产：`ContentSubmitted`、`ContentPublished`（含 SearchDoc）、`ContentTakenDown`、`ContentClosed`
- [ ] 3.9 注册 `EntityResolver`（content.* 四类）、`ReviewSubjectProvider`
- [ ] 3.10 字数与格式校验（PRD 8.9：标题 ≤50 字、核心优势/关键点 ≤500 字、简介 ≤2000 字等，提交级校验定位第一个未填必填项）

## 4. social（R1）

- [ ] 4.1 迁移：`soc_follow`（唯一键 follower_id+followee_ref）、`soc_favorite`、`soc_follow_unread`
- [ ] 4.2 接口：`POST/DELETE /follows`、`POST /favorites`、`GET /favorites?type=`（六类分 Tab）、`GET /follows|followers`（两页签，不设互相关注统计——待确认 #19 结论）
- [ ] 4.3 关注=订阅：被关注者发新内容时产生通知事件（供 notice 消费）；`FollowerUnread` 未读数
- [ ] 4.4 事件：`Followed/Unfollowed`；目标统一 EntityRef，经 `EntityResolver` 回填展示

## 5. discovery（R5，规则版）

- [ ] 5.1 迁移：`dsc_search_doc`、`dsc_match_candidates`（4.2.19，唯一键 user_id+candidate_ref）
- [ ] 5.2 事件订阅投影：`ProfileUpdated/OrgCardUpdated/ContentPublished/TakenDown/CertificationApproved/MemberActivated/Expired/Followed`；下架/关闭事件同步删除或置灰；版本防旧事件覆盖
- [ ] 5.3 投影回放工具：读模型可随时由事件回放重建
- [ ] 5.4 匹配候选预计算：画像更新事件触发倒排求交重算 Top-K 写入 `dsc_match_candidates`；「先给后要」注册即按粗标签给 3 条
- [ ] 5.5 评分公式：需求供给互补 40%+行业 25%（同子行业>同行业）+区域 20%+身份互补 15%；权重 SettingsSchema 配置化带版本；同分按会员等级（企业版>SVIP>VIP>普通），不设推荐位/插队
- [ ] 5.6 推荐理由规则模板生成（不依赖 LLM）：`GET /matches/{id}/explain`
- [ ] 5.7 搜索：`GET /search?q=&type=` 六类内容按名称检索（PG pg_trgm/中文分词，经 Search 端口抽象）；标签筛选与关键词独立可用
- [ ] 5.8 bff/miniapp 聚合：`GET /feed` 单次返回卡片流+画像进度+未读数；`POST /feed/refresh` 换一批；详情页聚合（名片+Badges+会员等级+关注状态）

## 6. trust（完整链路）

- [ ] 6.1 迁移：`trs_report`（4.2.4）、`trs_violation_score`、`trs_restriction`
- [ ] 6.2 举报：`Report` 接口 + 举报原因枚举；举报 → 人工复核队列（24h SLA）
- [ ] 6.3 复核结论：属实 → 发 `ReportConfirmed`（content 下架）+ 违规记分；3 次记分 → 限制发布 7 天；严重者封号（调 identity）
- [ ] 6.4 content/profile 注册 `ReviewSubjectProvider` 联调：取文本/图片、结论落回
- [ ] 6.5 高风险类目（金融/医疗/政府项目）人工抽检入队规则
- [ ] 6.6 事件：`ModerationPassed/Rejected`、`ReportConfirmed`、`ViolationRecorded`、`AccountRestricted`
- [ ] 6.7 审核队列 SLA 排序与积压监控指标

## 7. trade（R1）

- [ ] 7.1 迁移：`trd_order`（biz_type/biz_ref/amount_fen/price_snapshot/pay_expire_at/idempotency_key 唯一）、`trd_payment`（out_trade_no 唯一）、`trd_refund`（幂等键=payment_id+reason）、`trd_recon`
- [ ] 7.2 下单/支付：`POST /orders`、`POST /orders/{id}/pay`；创建/支付类接口强制 `Idempotency-Key`
- [ ] 7.3 微信支付回调：验签 + 幂等（重复/乱序回调测试）；验签失败记审计（防重放）
- [ ] 7.4 订单状态机 #15（待支付→已支付/已关闭；超时关闭走 SchedulerJob）+ 测试
- [ ] 7.5 `OrderFulfiller` 注册表：按 biz_type 履约；履约幂等（订单 ID 去重）、失败重试、超阈告警、后台手动补偿入口
- [ ] 7.6 退款：仅后台发起（用户端无入口）；原路退回；`RefundSucceeded` 事件
- [ ] 7.7 每日对账 SchedulerJob：`trd_recon` 差异记录与告警
- [ ] 7.8 事件：`OrderCreated/OrderPaid/OrderClosed/RefundSucceeded`
- [ ] 7.9 tests/trade：回调重复/乱序、支付幂等、对账差异用例

## 8. membership（R3）

- [ ] 8.1 迁移：`mbr_member`、`mbr_sub_account`（同一时间唯一归属）；价格入 settings（年费锚点、折扣系数 3 年 0.9/5 年 0.8、SVIP 预留档位不上架）
- [ ] 8.2 `PriceQuote(level,years)`：多年价=年费×年限×系数，同时返回原价与折后价；年限 1/3/5/自定义
- [ ] 8.3 下单：调 trade.CreateOrder（biz_type=member，price_snapshot 存单价/年限/系数）
- [ ] 8.4 履约：注册 `OrderFulfiller(biz_type=member)`；`OrderPaid` → 开通/续期（幂等）
- [ ] 8.5 状态机 #12（未开通→生效中→已过期）；到期回落 SchedulerJob（企业版主+子同步回落，子账号归属保留 30 天）
- [ ] 8.6 企业版：仅已认证企业可开通（订阅 `CertificationApproved` 判资格）；3 个子账号按手机号邀请（须已注册已实名）、共享权益不单独计费；非企业账号开通企业版 denied 记审计
- [ ] 8.7 退款金额计算：`RefundAmount(order)`=剩余完整年数×年费×实付折扣系数（取订单快照）；支付成功即生效不支持退款（开通页明示）；重复支付多扣全额原路退回
- [ ] 8.8 事件：`MemberActivated/MemberExpired/SubAccountChanged`；identity 订阅递增 authz_ver；discovery 订阅刷新权重
- [ ] 8.9 CapabilityProvider（会员生效中、等级）；会员权重输出给 discovery 同分排序

## 9. msg（R1，异步单聊）

- [ ] 9.1 迁移：`msg_session`（(min_uid,max_uid) 唯一）、`msg_session_member`、`msg_message`（按月分区）、`msg_read_cursor`
- [ ] 9.2 `POST /sessions`（EnsureSession 单聊，发起=会员权益经 policy）；`POST /sessions/{id}/messages`、`GET /sessions/{id}/messages?cursor=`、`POST /sessions/{id}/read`、`GET /sessions`
- [ ] 9.3 未读数=会话最大序号−已读游标（SQL 幂等，免 Redis 计数器）；`unread_count` 冗余可重建
- [ ] 9.4 增量同步协议：cursor 拉取；聊天记录留存 ≥6 个月，超期分区归档对象存储（保留可检索元数据）
- [ ] 9.5 消息异步内容检测：注册 `ReviewSubjectProvider`（聊天），`MessageSent` 事件
- [ ] 9.6 非成员拉取会话 denied 记审计；`Realtime` 适配器 Noop

## 10. 小程序主链路

- [ ] 10.1 shell：5 Tab（广场/圈子/猫立方/消息/我的）；主包仅 Tab 壳+广场+消息，其余分包
- [ ] 10.2 登录注册一体化（微信授权主推、协议勾选、注册弹窗、身份选择页三条动线落名片编辑页）
- [ ] 10.3 广场 Tab：搜索框、类型筛选（默认全部）、六类卡片流无限滚动、底部「换一批」；未登录可浏览
- [ ] 10.4 完善资料提示条：五项缺一即显示、填全自动消失、清空重现、随内容滚动不悬浮、不引导实名、统一称「完善资料」；点击/上滑进对应名片编辑页
- [ ] 10.5 搜索页（输入/历史/推荐词/分类结果/空态「没有找到相关内容」）
- [ ] 10.6 六类详情页共用壳（区块按详细设计 5.6）；底部「关注Ta / 联系Ta（会员）」；非会员点联系Ta 弹引导开通弹窗（不展示联系方式）
- [ ] 10.7 四类发布表单（字段各自独立，项目 8/产品 11/会议 9/活动 11 字段；会议与活动共用「时间/地点/报名」组件但字段不合并）；发布页嵌 ai-entry 占位（未装载自动隐藏）
- [ ] 10.8 名片编辑页（个人 10 字段/组织 10 字段，详细设计 5.12）；我的收藏/发布/关注粉丝页
- [ ] 10.9 会员开通页：状态与有效期提示、协议、年限选择、原价与折后价同显、支付前明示「开通后不支持退款」、企业版入口（仅已认证企业）
- [ ] 10.10 消息列表（系统通知置顶+单聊，未读数与 Tab 红点、3 秒轮询退避）与单聊会话页
- [ ] 10.11 共享组件：ContentCard 卡片注册表（六类共用壳）、PolicyGate（受限灰显+Reason+解锁动作）、TaxonomyPicker（两级行业）、Uploader、CursorList、EmptyState、SubscribeAsk、PayButton
- [ ] 10.12 空态/加载/错误态全表落地（详细设计 5.14：骨架屏、页面级重试、文案不暴露技术细节）

## 11. admin 基础管理

- [ ] 11.1 各模块 admin/ 接口与 AdminSection：用户列表与封禁（identity）、内容管理（content）、审核队列（trust）、订单（trade）、会员列表（membership）
- [ ] 11.2 admin-web shell 登录与布局；对应 features 基础页面（列表+操作，RBAC 细化在 p4）

## 12. 测试与门禁

- [ ] 12.1 权限矩阵用例：PRD 3.2/3.3 主链路相关格（游客浏览/注册用户发布收藏关注/会员联系方式核心优势单聊/未实名不影响浏览搜索被检索）
- [ ] 12.2 状态机测试：#1、#2、#12、#15 全表合法/非法转移
- [ ] 12.3 契约测试：本阶段全部 OpenAPI 路径与事件 Schema 生产/消费一致
- [ ] 12.4 端到端闭环测试：注册 → 补画像 → feed 有匹配 → 详情 → 开通会员（mock 支付回调）→ 联系Ta 建会话 → 发消息 → 对方未读数正确
- [ ] 12.5 首屏性能：feed 单次聚合接口 + 预计算候选，首屏 ≤3 秒（PRD 10.4.2）

## 13. 退出标准

- [ ] 13.1 「找对人 + 联系上 + 付费开通」闭环跑通（12.4 自动化通过）
- [ ] 13.2 状态机与权限矩阵测试全绿；archcheck 与契约测试 CI 通过
- [ ] 13.3 主链路全部页面按详细设计第 5 章走查通过（空态/错误态/PolicyGate 无例外）
