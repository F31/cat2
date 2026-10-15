# p2-main-loop · 任务清单

## 1. profile（R1）

- [x] 1.1 迁移：`prf_card`（详细设计 4.2.8 全字段：subject_type_code/三层画像字段/contact_phone_enc/visibility_settings/profile_tier/search_doc_version/locked_fields/owner_space_id）
- [x] 1.2 名片 CRUD：`GET/PUT /cards/me`、`GET /cards/{ref}`（他人名片按对外展示设置过滤）、`POST /cards/me/share`
- [x] 1.3 画像完成度 `GET /cards/me/completion`：第二层五项（区域/行业/合作需求/合作提供/简介）任一为空即未完成；与实名无关
- [x] 1.4 组织名片资质证明多图上传（写入 qualification_file_ids，增删接 media.Bind/Unbind）
- [x] 1.5 认证主体信息锁定：profile 订阅 `verify.CertificationApproved.v1`（topic 字面量，不 import 高阶 contract）→ 写 `locked_fields`（实名锁 name；企业认证锁 name/subject_type_code/region_code）+ 更新拦截 060003；`TestIntegrationRealnameLocksCard` 覆盖
- [x] 1.6 事件生产：`ProfileUpdated`/`OrgCardUpdated`（含 MatchProfile，不含敏感字段，带 search_doc_version；与名片同事务入 Outbox）
- [x] 1.7 注册扩展点：`ReviewSubjectProvider`（名片文本/头像，wire 装配进 trust）、`EntityResolver`（user/org）
- [x] 1.8 domain 单元测试：完成度判定、展示设置过滤、认证锁定、长度校验

## 2. verify（R2，仅实名）

- [x] 2.1 迁移：`vfy_certification`（4.2.11，含实名姓名/证件号密文列）、`vfy_badge_def`、`vfy_badge_grant`（affiliation 表随身份核验后续增量补）
- [x] 2.2 实名认证：真实姓名+身份证号+协议勾选；`ports.RealnameVerify` 二要素实时核验（含开发态实现），不上传证件照；人脸核身预留开口
- [x] 2.3 状态机 #1 实名（未实名→核验中→已实名/核验失败，失败可重提）`StateMachineDef` + 表驱动测试（合法/非法转移）
- [x] 2.4 核验通过回写 `idn_user.realname_status`（经 identity contract `MarkRealname`，不读他人表）并授予「实名」Badge
- [x] 2.5 注册 CapabilityProvider（已实名读 vfy 表；已认证/组织管理员本阶段恒否）；`ReviewSubjectProvider`（认证材料）开口已在 wire 注册
- [x] 2.6 contract：`Certify`、`BadgesOf(ref)`、`CertsOf`、`IsRealname`；事件 `CertificationApproved/Rejected` Schema

## 3. content（R2）

- [x] 3.1 迁移：`cnt_item` 信封表（4.2.13）+ `cnt_project/cnt_product/cnt_meeting/cnt_event` 四张类型子表（4.2.14，类型化列筛选排序）
- [x] 3.2 `ContentKind` 注册表：字段 schema 与校验、卡片字段、详情字段、会员可见字段清单、审核文本/图片提取、SearchDoc 映射；四类 Kind 各自注册
- [x] 3.3 四类发布接口：`POST /contents`（kind 区分）、`POST /contents/{id}/submit`、`GET /contents/{id}`、`GET /contents/mine?kind=`、`POST /contents/{id}/close`
- [x] 3.4 发布门槛策略：已实名（经 policy CapabilityProvider）；未实名拦截（审计点待接 audit）
- [x] 3.5 状态机 #2（审核中→展示中→已下架；已下架不可自行恢复）+ 表驱动测试（合法/非法转移）
- [x] 3.6 订阅 `trust.ReportConfirmed` 驱动下架（已打通，经 Outbox 投递）；`ModerationPassed/Rejected` 提交时同步驱动（trust 已发事件，推送式切换留后续）
- [x] 3.7 会员可见字段：`Get` 输出按 policy 能力过滤核心优势/合作联系，返回 `unlock_reason`
- [x] 3.8 事件生产：`ContentSubmitted`、`ContentPublished`（含 SearchDoc）、`ContentTakenDown`、`ContentClosed`
- [x] 3.9 注册 `EntityResolver`（content.* 四类）、`ReviewSubjectProvider`（wire 装配进 trust）
- [x] 3.10 字数与格式校验（标题 ≤50、核心优势/关键点 ≤500、简介 ≤2000；`Kind.Validate` 定位第一个未填必填项）

## 4. social（R1）

- [x] 4.1 迁移：`soc_follow`（唯一键 follower_id+followee_type+followee_id）、`soc_favorite`、`soc_follow_unread`
- [x] 4.2 接口：`POST/GET/DELETE /follows`、`GET /followers`、`POST/GET/DELETE /favorites`（六类分 Tab）、`GET /follow-unread`
- [x] 4.3 关注=订阅：订阅内容发布 → 关注者未读 +1 并产生 `FolloweePublished` 通知事件（供 notice 消费）
- [x] 4.4 事件：`Followed/Unfollowed/FolloweePublished`；目标统一 EntityRef，经 `EntityResolver` 回填 title/cover

## 5. discovery（R5，规则版）

- [x] 5.1 迁移：`dsc_search_doc`、`dsc_match_candidates`（唯一键 user_id+candidate_ref）+ `dsc_projection_meta`（pg_trgm 扩展）
- [x] 5.2 事件订阅投影：`ProfileUpdated/OrgCardUpdated/ContentPublished/TakenDown/Closed/CertificationApproved/MemberActivated/Expired/Followed/UserRegistered/UserDeactivated`；下架/关闭置灰；`search_doc_version` 防旧事件覆盖
- [x] 5.3 投影回放工具：`Module.Rebuild()` 清空后按 Outbox 事件顺序重放（经 `OutboxReader` 端口，不触碰 sys_ 表）
- [x] 5.4 匹配候选预计算：画像更新触发重算 Top-K 写入 `dsc_match_candidates`；`discovery.refresh` 周期收敛跨用户新增；「先给后要」注册即给 ≥3 条
- [x] 5.5 评分公式：需求供给互补 40+行业 25（同子行业>同行业）+区域 20+身份互补 15；权重 SettingsSchema 配置化；同分按会员等级→近期活跃
- [x] 5.6 推荐理由规则模板生成（不依赖 LLM）：`GET /matches/{ref}/explain`
- [x] 5.7 搜索：`GET /search?q=&type=` 六类按名称检索（`dsc_` 读模型 + pg_trgm，经 discovery 内 `Repo` 抽象）；标签筛选与关键词独立可用（外部引擎切换走 ports.Search 留后续）
- [x] 5.8 bff 聚合层 `internal/bff`：`GET /feed` 单次返回卡片流+画像进度+未读数；`POST /feed/refresh`（offset 换一批）；`GET /aggregate/detail`（名片+Badges+会员等级+关注状态；内容类带归属人名片）
- 备注：新增内核事件投递 worker `kernel.outbox_relay`（Outbox→订阅）；修 identity 事件 user_id 未按契约输出字符串的 bug

## 6. trust（完整链路）

- [x] 6.1 迁移：`trs_report`（4.2.4）、`trs_violation_score`、`trs_restriction`（含 `trs_review_task`/`trs_sensitive_word`/`trs_moderation_record`）
- [x] 6.2 举报：`GET /report-reasons` + `POST /reports`（原因枚举）+ 入人工复核队列（24h SLA）
- [x] 6.3 复核结论：`ResolveReview` 属实 → 发 `ReportConfirmed`（content 订阅下架）+ 违规记分；3 次 → 限制发布 7 天（`AccountRestricted`）；严重封号走 `Restrict(days<=0)`
- [x] 6.4 content/profile/verify 注册 `ReviewSubjectProvider` 联调：`Extract` 取素材、`OwnerOf` 归属、`ApplyVerdict` 落结论
- [x] 6.5 高风险类目（金融/医疗/政府项目关键词）人工抽检入队（source=5）
- [x] 6.6 事件：`ModerationPassed/Rejected`、`ReportConfirmed`、`ViolationRecorded`、`AccountRestricted`（JSON Schema + `make gen`）
- [x] 6.7 审核队列 SLA 到期排序（`ReviewQueue`）与积压指标（`SLAMetrics`，admin 路由）

## 7. trade（R1）

- [x] 7.1 迁移：`trd_order`（idempotency_key 部分唯一）、`trd_payment`（out_trade_no 唯一）、`trd_refund`（幂等键=payment_id+reason）、`trd_recon`、`trd_fulfillment`、`trd_apportion_group`/`trd_pledge`
- [x] 7.2 下单/支付：`POST /orders`（强制 `Idempotency-Key`，重复键返回首单）、`POST /orders/{id}/pay`、`GET /orders/{id}`
- [x] 7.3 微信支付回调 `POST /pay/notify`：验签（`ports.Pay`）+ 幂等（重复回调不重复置状态/不重复发事件）；验签失败返回 080005（审计钩子待 audit 存储落地）
- [x] 7.4 订单状态机 #15（待支付→已支付/已关闭）+ 表驱动测试；超时关闭走 `trade.close_expired` SchedulerJob
- [x] 7.5 `OrderFulfiller` 注册表 + `trd_fulfillment`：按 biz_type 履约、失败退避重试、≥8 死信、后台补偿入口 `POST /admin/v1/fulfillments/{orderID}/retry`
- [x] 7.6 退款：仅后台 `POST /admin/v1/refunds`（用户端无入口）；幂等（payment+reason）；原路退回；`RefundSucceeded` 事件
- [x] 7.7 每日对账 SchedulerJob（`trade.recon` → `trd_recon`；dev 以订单侧汇总，渠道账单对接留后续）
- [x] 7.8 事件：`OrderCreated/OrderPaid/OrderClosed/RefundSucceeded`（Schema + `make gen`）
- [x] 7.9 状态机表驱动测试 + 集成测试（下单幂等、回调重复幂等、验签失败、退款幂等）

## 8. membership（R3）

- [x] 8.1 迁移：`mbr_member`、`mbr_sub_account`（同一时间唯一归属）；价格入 settings（年费锚点、折扣系数 3 年 0.9/5 年 0.8、SVIP 预留档位不上架）
- [x] 8.2 `PriceQuote(level,years)`：多年价=年费×年限×系数，同时返回原价与折后价；年限 1/3/5/自定义
- [x] 8.3 下单：调 trade.CreateOrder（biz_type=member，price_snapshot 存单价/年限/系数）
- [x] 8.4 履约：注册 `OrderFulfiller(biz_type=member)`；`OrderPaid` → 开通/续期（幂等，按订单 ID 幂等）
- [x] 8.5 状态机 #12（未开通→生效中→已过期）+ 到期回落 SchedulerJob；企业版主账号过期 → 子账号同步回落（`ExpireSubsByMaster` 置 status=2），归属记录保留 30 天（`PurgeRemovedSubs` 随 `membership.expire` 清理）；`TestIntegrationEnterpriseSubExpiry` 覆盖
- [ ] 8.6 企业版门槛：开通时校验 identity `identity_type=2` + verify「已认证」能力（当前恒否→denied）；订阅 `CertificationApproved` 判资格、按手机号邀请、denied 记审计 **阻塞于企业认证（cert_type=4）未在 p2 verify（仅实名）实现**，随 verify 企业/机构认证阶段接通（审计存储已就绪，可直接 `audit.Record(ResultDenied)`）
- [x] 8.7 退款金额计算：`RefundAmount(order)`=剩余完整年数×年费×实付折扣系数（取订单快照）；用户端无退款入口（仅后台）
- [x] 8.8 事件 `MemberActivated/MemberExpired/SubAccountChanged` 已发；identity 订阅递增 authz_ver 已接通；discovery 订阅 `MemberActivated/Expired` → `SetMemberLevel` 落投影（修复 `dsc_search_doc` 查询 `'user:'||$1` 参数类型歧义导致等级更新失败）
- [x] 8.9 CapabilityProvider（`member.active`/`member.enterprise`）已注册；会员等级作为 discovery 同分排序权重（企业版>VIP>普通）

## 9. msg（R1，异步单聊）

- [x] 9.1 迁移：`msg_session`（(min_uid,max_uid) 唯一偏索引）、`msg_session_member`、`msg_message`（按月分区 + default）、`msg_read_cursor`、`msg_archive_meta`
- [x] 9.2 `POST /sessions`（EnsureSession 单聊，发起=会员权益经 policy）；`POST /sessions/{id}/messages`、`GET /sessions/{id}/messages?cursor=`、`POST /sessions/{id}/read`、`GET /sessions`
- [x] 9.3 未读数=会话最大序号−已读游标（SQL 幂等，免 Redis 计数器）；`unread_count` 冗余可重建
- [x] 9.4 增量同步 cursor 拉取、分区自动创建、超期(>6 月)归档元数据登记；归档补齐：分区导出 JSONL → gzip → 对象存储（`Storage.Put`，未配置 endpoint 时本地 FS 回退）→ 标记 `status=1/object_key` → detach（上传失败不丢数据、保留待重试）；`TestIntegrationMsgArchive` 覆盖（可重复执行）
- [x] 9.5 已注册 `ReviewSubjectProvider`（msg.message）+ 发 `MessageSent` 事件；trust 订阅 `msg.MessageSent.v1` 异步机检（敏感词/文本机检命中入审核队列，不阻塞投递）；补齐敏感词库加载（wire 启动 + `trust.load_words` 周期 Job）；`TestIntegrationMessageModeration` 覆盖
- [x] 9.6 非成员拉取/发送 denied（070002）落审计：kernel audit 哈希链 + `adapters/pg` `sys_audit` 存储（`LastHash`/`Append`，复用业务事务或独立写），wire 注入 `Recorder`，msg 在 `sync/send/markread` 拒绝时记 `ResultDenied`；`TestIntegrationDeniedPullAudited` 覆盖；`Realtime` 适配器 Noop 已接

## 10. 小程序主链路

- [x] 10.1 shell：`app.config.ts` 5 Tab（广场/圈子/猫立方/消息/我的）+ 主包壳+广场+消息 + 分包（content.detail/member.open/search）；Taro 配置与 `taro build --type weapp` 通过
- [x] 10.2 登录注册一体化：`ensureLogin` 微信授权 + 令牌持久化；登录引导页 `pages/auth/enter` 三条动线齐备——协议勾选（未勾选拦截）、《用户协议/隐私政策》链接、注册确认弹窗（含身份选择 个人/企业）、手机号快速验证授权；`/auth/wechat` 新增 `identity_type`（仅注册新用户生效，老用户忽略，缺省 1），企业版开通链路就此可达；协议/隐私内容页占位
- [x] 10.3 广场 Tab：搜索框入口、匹配卡片流、底部「换一批」（offset 下翻）；游客可搜索/看详情
- [x] 10.4 完善资料提示条：`ProfilePromptBar`（五项缺一显示、填全消失、不引导实名、无关闭、随页滚动）
- [x] 10.5 搜索页：关键词 + 六类 Tab 筛选、空态「没有找到相关内容」
- [x] 10.6 详情页共用壳（详细设计 5.6）：基本信息/认证标识+会员等级/标签/需求供给/核心优势/简介各区块；内容类按 `fields` 渲染并展示 `member_gate` 未解锁字段（引导开通）+ 发布方；底部「关注Ta / 联系Ta（会员）」——游客可看（`aggregate/detail` 登录态可选，following=false/member 省略）、「联系Ta」揭示手机号（`aggregate/contact`，非会员 120007 引导开通，弹层内可「发消息」进单聊）
- [x] 10.7 四类发布表单 `pages/content/publish`（项目/产品/会议/活动字段独立；标题+封面+区域+行业/需求/供给标签+按类字段；提交复核）；ai-entry 占位待补（随 ai 模块二期）
- [x] 10.8 名片编辑页 `pages/card/edit`（第二层五项+增值画像+联系方式，保存）与我的发布 `pages/content/mine` 已实现；我的收藏 `pages/social/favorites`（六类分 Tab+取消收藏）与关注/粉丝 `pages/social/follows` 已实现，mine 页入口已接通
- [x] 10.9 会员开通页：年限选择、原价与折后价同显、协议勾选、「开通后不支持退款」明示、企业版入口（identity_type=2）
- [x] 10.10 消息列表（单聊会话+未读）+ 系统通知置顶（占位，通知中心随 notice 模块 p3 接入）+ 单聊会话页 `pages/msg/chat`（cursor 增量同步、发送本地追加、3 秒轮询无新消息指数退避至 30 秒、自动已读上报、滚动定位最新消息）；详情「联系Ta」与消息列表均已接通会话页
- [x] 10.11 共享组件：`ContentCard`/`PolicyGate`/`ProfilePromptBar`/`StateView`(EmptyState) 已落地；新增 `TaxonomyPicker`（多选上限/父级联动/可搜索，`TagPicker` 为其兼容别名）、`Uploader`（预签名上传→回填 file_id，JPG/PNG≤5MB；media 上传接口对齐契约 JSON body）、`CursorList`（游标分页+空/载/错态）、`SubscribeAsk`（关注=订阅提示）、`PayButton`（下单→取参→调起微信支付，dev 无签名仅确认订单）；名片头像、发布封面、会员开通、关注提示、消息列表已接入
- [x] 10.12 空态/加载/错误态落地（详细设计 5.14）：统一 `StateView` 组件（loading/empty/error 可点击重试），已应用于广场/搜索/消息/我的/详情/我的发布/会话页；骨架屏与更细粒度错误分类后续优化

## 11. admin 基础管理

- [x] 11.1 admin 接口：用户列表/封禁解封（identity `/api/admin/v1/users*`）、内容列表/下架（content `/api/admin/v1/contents*`）、订单列表（trade `/api/admin/v1/orders`）、会员列表（membership `/api/admin/v1/members`）；审核队列（trust）与退款/履约补偿（trade）此前已具备。（RBAC 细化在 p4；独立 admin OpenAPI 契约随 admin-web 落地补）
- [x] 11.2 admin-web（Vite+AntD+Router）：登录页、侧栏+顶栏布局、概览(SLA) 与用户/内容/审核队列/订单/会员基础页面（列表+操作；驳回必填原因）；`tsc --noEmit` 与 `vite build` 通过。（菜单暂前端声明，模块 AdminSection 声明式组装与 RBAC 细化在 p4）

## 12. 测试与门禁

- [x] 12.1 权限矩阵用例：`TestIntegrationPermissionMatrix`（游客浏览/未实名仍被检索/单聊=会员权益）
- [x] 12.2 状态机全表：#1 verify、#2 content、#12 membership、#15 trade 合法/非法转移表驱动测试
- [x] 12.3 契约测试：`TestEventContractsDeclared`（27 个生产事件均有 Schema 且 topic 一致）+ `TestOpenAPIDeclaresP2Paths`（主链路路径齐备）
- [x] 12.4 端到端闭环：`TestIntegrationClosedLoop`（注册→补画像→feed 命中→详情→开通会员→联系Ta 建会话→发消息→未读=1）
- [x] 12.5 首屏性能：`GET /feed` 单次聚合 + 预计算候选已就绪；新增 `TestIntegrationFeedLatency` 守后端读路径（10 次均值 < 300ms，实测 ~2.4ms；端到端 ≤3s 含前端渲染与网络）

## 13. 退出标准

- [x] 13.1 「找对人 + 联系上 + 付费开通」闭环跑通：`TestIntegrationClosedLoop`（注册→补画像→feed 命中→详情→开通会员→联系Ta 建会话→发消息→未读=1）通过
- [x] 13.2 状态机（#1/#2/#12/#15）与权限矩阵测试全绿；`make lint`（archcheck 依赖阶梯/表前缀/infra 禁令/bff 只读 + go vet）与契约测试（27 事件 Schema、OpenAPI P2 路径）通过；全量 `go test -tags=integration ./...` 连跑 3 次绿
- [x] 13.3 主链路页面走查（代码级）：广场/搜索/消息/我的/详情/发布/我的发布/名片编辑/会员开通/收藏/关注粉丝/单聊/登录引导/实名认证 各页均具备 加载/空/错误 态（`StateView`/`CursorList`）与门槛引导（`PolicyGate`：会员联系方式/核心优势 `member.required`、发布实名 110004→实名页、企业版 120003→认证引导）；游客可看详情与搜索
