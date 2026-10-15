# p3-collaboration · 任务清单

## 1. cube：空间与成员

- [x] 1.1 迁移：`cub_cube`（4.2.15 全字段含 plan_level/plan_expire_at/owner_space_id）、`cub_member`、`cub_plan`、`cub_invite`
- [x] 1.2 创建猫立方：组名/区域/行业/环节/方向/有啥/优势/合作；创建者即猫王且必须再指定 1 名猫蜜（不可为自己）；创建者与猫蜜均须已实名；可关联一个组织复用字段（保持独立实体）
- [x] 1.3 邀请制加入：猫王+猫蜜邀请（邀请码；前端据此生成卡片/二维码，7 天有效）；加入须已实名且未被该立方移除过；被移除不可自行重进（`CanJoin` + 终态 #6）；「需对方确认/可配免确认」配置化留待 settings（§3）
- [x] 1.4 猫王转让（`cube:leader:transfer` 落审计；角色变更递增 authz_ver）；成员角色：猫王/猫蜜/普通成员/共建成员（退出/移除亦记审计）
- [ ] 1.5 配额：档位上限常量与创建/加入时校验已实现（可加入立方数、单立方成员数）；QuotaDef 声明式化与 settings 后台可配随 §3 订阅落地（口径按 p0 #15 结论）
- [x] 1.6 状态机 #5 猫立方（正常/封禁/已解散）、#6 成员（正常→退出/被移除，被移除为终态）+ 表驱动测试
- [x] 1.7 contract：`CubeBrief`/`RoleOf`/`Invite`/`JoinByCode`/`Transfer`/`Dissolve`/`RemoveMember`/`Leave`；注册 `CapabilityProvider`（`cube.member`）与 `EntityResolver`（cube；cube.project 随 §4）
- [x] 1.8 事件：`CubeCreated/MemberJoined/MemberRemoved/CubeDissolved`（Schema 已入 `contracts/events/`）；成员变更同步递增相关用户 authz_ver

## 2. cube：协作实体

- [x] 2.1 迁移：`cub_project`、`cub_task`、`cub_report`、`cub_research`、`cub_project_link_req`（4.2.16）；迁移版本改为内容哈希（幂等 DDL 新增表自动应用）
- [x] 2.2 研究模块：项目列表字段、分工待办（任务）、研究记录→决策结论、关闭/重开（状态机 #7）
- [x] 2.3 承接模块：项目（type=2，含合作方客户）+ 任务分配 + 进度跟踪（关闭/重开同 #7）
- [x] 2.4 任务四态 #8：待办→进行中→待确认→已完成；负责人提交进度转待确认；确认人（默认猫王+猫蜜，可单任务指定）确认通过转已完成、驳回须填原因退回进行中（未填 130011）；已完成可重开（记操作人与时间）；表驱动测试（逾期提醒不自动通过由提醒任务承担，状态不被自动改变）
- [x] 2.5 内部工作安排：任务可不挂项目（project_id 可空）
- [x] 2.6 重点客户报备 #9：关联项目产品/客户名称（联系方式加密）/类型/区域/预计金额万元/说明 ≤200 字；确认/驳回（须填原因、可重提，状态机 #9）；重复报备 `duplicate=true` 提示不拦截；入口在项目详情页按状态分组（前端 §9）
- [x] 2.7 事件：`ProjectCreated/Closed/Reopened`、`TaskAssigned/Submitted/Confirmed`、`ReportSubmitted/Confirmed/Rejected`（Schema + 契约测试）

## 3. cube：订阅与分摊

- [x] 3.1 订阅三档入 settings（`cube.plan.*.price_fen/cube_limit/member_limit` 全后台可配）；`GET /cubes/plans`、`POST /cubes/{id}/plan`（升降档，降档校验成员上限）
- [x] 3.2 订阅到期回落 SchedulerJob（`cube.plan_expire`，降回基础档，`ListExpiredPlans`）
- [x] 3.3 分摊发起：猫王选档设人数 N（2≤N≤档上限）不设价，每人=档价÷N；默认 7 天截止；名单 `member_ids` 随快照（续费可重设）
- [x] 3.4 trade 分摊：`trd_apportion_group`/`trd_pledge`；认筹支付成功在同一 trade 事务内 `SELECT ... FOR UPDATE` 锁 group 行计数，达 N 置「已成团」并关闭入口；超额笔提交后原路退回；并发测试证明不超额
- [x] 3.5 开通：cube 注册 `OrderFulfiller(biz_type=cube_plan)`；订单支付履约 + 订阅 `trade.ApportionFormed` → 开通订阅（幂等）
- [x] 3.6 成团成员自动成为立方成员（role=4 共建成员，`AddMember` 不受成员上限）
- [x] 3.7 未成团：`trade.apportion_expire` 到期扫描逐笔幂等全额退回，全部完成才终结（`trade.ApportionFailed`）；截止前可退出退款（`/apportion/exit`）
- [x] 3.8 tests：并发成团（3 笔同时支付→恰好 2 计入、超额退款）、未成团退款幂等（重复执行不重复退）；重复回调沿用 trade 幂等单测

## 4. cube：项目关联（引用 + 本地状态）

- [x] 4.1 `cub_project.content_project_id` 可空无外键；同一平台项目可进多个立方各一行、状态独立（A 关闭不影响平台展示与 B）
- [x] 4.2 引入规则：本人内容直接加；猫王/猫蜜引入他人内容 → 项目方确认（target_role=1）；普通成员发起 → 猫蜜确认（target_role=2）；落 `cub_project_link_req`（待确认/已同意/已拒绝）
- [x] 4.3 平台项目下架：立方项目 `platform_status` 经 `EntityResolver` 解析（published→active；下架/不可解析→taken_down），卡片显示「平台已下架」不删除
- [x] 4.4 立方内新建未发布项目 content_project_id 为空；「发布到平台」调 `content.CreateFromCube`（新建平台草稿）回写 ID
- [x] 4.5 状态机 #7 立方内项目（进行中→已关闭，可重开）+ 测试

## 5. circle：圈子生命周期

- [x] 5.1 迁移：`cir_circle`（含 type/duration_days/end_at/recommendable_at）、`cir_member`、`cir_invite`（另附 §6 的 `cir_demand/cir_feed/cir_mount`）
- [x] 5.2 建圈：已实名门槛；常设/临时两类，临时 7/30/90/180 天（默认 30）；创建者即圈主；`recommendable_at=now()+7d`（新圈 7 天不进推荐流）
- [x] 5.3 配额：建圈 ≤1/日、同时最多 3 个（进行中计数即释放）；加入不设上限（走 count 校验；声明式 QuotaDef 化留待后台配置页统一）
- [x] 5.4 加入四路：a 申请（`/apply` + 圈主 `ApproveApply`）；b 邀请链接（有效期=圈剩余存续期、生成即作废旧链、圈主与成员均可生成）；c/d 以 `via` 枚举预留（组织管理员/活动报名触发接口，p4/p3 活动接入时调用 `AddMember`）
- [x] 5.5 时效：`circle.end_due` 到期转「已结束」并发布 `CircleEnded`（只读、邀请链接过期由 expire 判定失效、通知由 notice 消费）；圈主可转常设（`/convert`）；解散（`/dissolve` 发布 `CircleDissolved`，二次确认在前端）
- [x] 5.6 状态机 #10 圈子、#11 圈内需求 + 表驱动测试（实现于 domain；单测随 #11 需求到期一并覆盖）

## 6. circle：圈内发布与挂载

- [x] 6.1 迁移：`cir_demand`、`cir_feed`、`cir_mount`（4.2.18）
- [x] 6.2 个人需求发布：标题 ≤30 字/内容 ≤500 字/类型/区域/行业/有效期默认 30 天/可见范围（普通仅本圈、会员可全平台）/联系方式加密且会员可见
- [x] 6.3 圈内动态（图文 `POST /feeds`，公告标记 `is_announcement`）
- [x] 6.4 配额：每人每日跨圈合计 3 条（需求+动态合并计数）、公告每圈每日 ≤3 条（仅圈主、不占个人额度）；单圈挂载 ≤50 条待 §6 挂载落地
- [x] 6.5 需求到期沉底标「已过期」（`circle.expire_demands` Job + `DemandExpired` 事件），一键续期 30 天（`POST /circles/demands/{did}/renew`）
- [ ] 6.6 挂载：仅我发布的项目/产品（EntityRef 引用不复制，点开进原详情页）；他人内容「申请挂载」需对方确认（approval_status）
- [ ] 6.7 「到猫立方里办」：四前提校验（来自本圈/需多方协作/已加入立方/有建项目权限）全满足才出现按钮否则置灰给引导；调 `cube.CreateProjectFromRef` 只带标题+正文+来源引用；订阅 `CubeProjectCreated` 回写「已转入猫立方办」，来源条目可点回原帖
- [ ] 6.8 圈内不聊天不评论不点赞不转发（无对应接口）；圈内动态/挂载可分享微信，落地页显示「来自 XX 圈」+ 加入入口
- [ ] 6.9 注册 `ReviewSubjectProvider`、`SearchDoc` 生产（CircleCreated/DemandPublished 事件，含 circle 类型供搜索与待确认 #22 的广场混排）

## 7. notice（R6）通知中心

- [ ] 7.1 迁移：`ntc_template`、`ntc_notice`、`ntc_subscribe_quota`、`ntc_digest`（4.2.20）
- [ ] 7.2 NoticeTemplate 数据驱动：事件→接收人规则→文案→渠道（站内/订阅消息/短信）→是否合并；PRD 11.3 共 36 条触达逐条映射为模板注册（清单在详细设计 8.3）
- [ ] 7.3 订阅全部业务事件执行模板；业务模块从不直接发通知（archcheck/评审检查）
- [ ] 7.4 订阅消息授权额度：`ntc_subscribe_quota`；在关注/加入猫立方/提交任务等上下文动作处申请授权（前端 SubscribeAsk 组件），发送时消耗；无额度降级站内未读并下次进入提示开启
- [ ] 7.5 同圈同日合并：`ntc_digest(circle_id,date,count)` upsert 计数 + 日结 SchedulerJob 发送（避免先判断再自增竞态）；本人不收
- [ ] 7.6 站内信兜底：任何事件至少一条站内；资金与审核类不可关闭；短信仅资金场景
- [ ] 7.7 接口：`GET /notices`、`POST /notices/{id}/read`、`GET/PUT /notice-preferences`、`POST /subscribe-quota/apply`
- [ ] 7.8 admin/：公告与模板管理 AdminSection（页面 p4）

## 8. msg 群聊补全

- [ ] 8.1 cube 注册 `SessionMemberResolver`：群成员=立方成员表；成员变更事件触发群成员同步
- [ ] 8.2 群聊会话：`related_ref` 关联立方/项目；1 项目 1 群聊；群聊名称=立方名·模块名（或项目名）；最后消息显示「发言人：内容摘要」
- [ ] 8.3 消息 Tab：系统通知置顶固定 + 单聊 + 群聊同列表，按最后消息时间倒序

## 9. 小程序协作页面

- [ ] 9.1 猫立方 Tab：空态（创建/输入邀请码）、创建（含指定猫蜜）、加入、详情四模块（研究/承接/内部工作安排/关于我们）、项目详情（分工待办/研究记录/集体沟通对话/报备入口按状态分组）、任务页（四态操作）、成员页、订阅与分摊页（认筹进度/退出/截止倒计时）共 15 页
- [ ] 9.2 圈子 Tab：我的圈子（我加入的/我创建的，进行中/已结束分区）、创建圈子、主题圈详情四区块（关于本圈/圈内动态/挂载内容/成员）、发布需求与动态、挂载、圈设置、邀请分享
- [ ] 9.3 非成员（含未登录）打开圈详情：仅见「关于本圈」+「申请加入」——分享链接/圈名片/二维码统一落地页
- [ ] 9.4 消息 Tab 群聊会话页
- [ ] 9.5 空态按详细设计 5.14（猫立方为空/我的圈子为空/报备列表为空等）

## 10. bff 聚合

- [ ] 10.1 猫立方首页聚合：立方列表+待办数+未读单次返回
- [ ] 10.2 圈详情聚合（非成员裁剪视图）：圈信息+成员数+圈主+加入状态

## 11. 测试与门禁

- [ ] 11.1 状态机测试：#5~#13 全表合法/非法转移（含逾期不自动通过的负向用例）
- [ ] 11.2 支付测试：并发成团、未成团退款幂等、履约重试（tests/trade）
- [ ] 11.3 配额测试：建圈/圈内发布/公告/挂载/立方成员上限
- [ ] 11.4 通知测试：36 条模板映射覆盖、合并逻辑、授权额度消耗与降级
- [ ] 11.5 契约测试：cube/circle/notice 新增事件 Schema 与 OpenAPI 路径

## 12. 退出标准

- [ ] 12.1 支付幂等与并发成团测试通过
- [ ] 12.2 状态机测试通过（#5~#13）
- [ ] 12.3 猫立方核心链路（建立方→邀请→项目→任务→报备→订阅/分摊）端到端跑通
- [ ] 12.4 圈子生命周期（建圈→加入四路→发布→到期/解散）端到端跑通
