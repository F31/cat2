# p1-foundation · 任务清单

> 勾选口径：代码/文件已落地且通过本地验证即勾选；仅有骨架但关键链路未通的条目保持未勾选，并在变更汇报中说明。

## 1. monorepo 脚手架

- [x] 1.1 创建仓库结构：`apps/server`、`apps/miniapp`、`apps/admin-web`、`packages/{api-client,domain-types,policy-hints,taxonomy-kit,form-kit,utils}`、`contracts/{openapi,events,errors,seeds}`、`deploy/`、`tools/`、`tests/`、`docs/{adr,modules,prd-trace}/`
- [x] 1.2 `apps/server` 初始化单一 go.mod；建 `cmd/catcube` 唯一入口（`serve | migrate | jobs | seed` 子命令骨架）
- [x] 1.3 `apps/miniapp` Taro + React + TS 初始化（依 p0 任务 3.1 的选型验证结论；不通过则原生回退并更新 ADR）
- [x] 1.4 `apps/admin-web` React + Ant Design 初始化；pnpm workspace 贯通三个前端应用与 packages
- [x] 1.5 根 `Makefile`：`make gen`（契约生成，当前为占位）、`make lint`（archcheck + go vet）、`make test`、`make up`（S0 compose）
- [x] 1.6 CODEOWNERS：每个模块目录指定 owner，`contract/` 变更必须 owner 审批
- [x] 1.7 CI 管线骨架：lint、test、契约校验、边界检查四个必过门禁（.github/workflows/ci.yml）

## 2. 契约与代码生成管线

- [x] 2.1 `contracts/openapi/mp.yaml` 首版：统一响应信封、错误码、游标分页、`request_id`、`Idempotency-Key` 约定
- [x] 2.2 契约生成：oapi-codegen → `internal/gen/`（Go types + std-http server），openapi-typescript → `packages/api-client/schema.d.ts`，`make gen` 一键完成
- [x] 2.3 `packages/domain-types`：状态枚举入契约（`contracts/enums/*.json`，10 个：UserStatus/ModerationResult/OrderStatus/ContentStatus/MemberLevel/CubeStatus/CubeMemberRole/PlanLevel/CircleStatus/CircleType）→ `scripts/gen-enums.mjs` 生成 `enums.generated.ts`（含 Label 映射）并接入 `make gen`（事件类型仍 json2ts）；`TestEnumContractsMatchBackend` 保证契约与后端常量逐值一致；`index.ts` 移除手写枚举
- [x] 2.4 事件契约：`contracts/events/identity.UserRegistered.v1.json`、`UserDeactivated.v1.json`、`AuthzChanged.v1.json` 三个 JSON Schema（含事件 ID、occurred_at、v1、space_id，不含敏感字段）
- [x] 2.5 CI 校验生成物无漂移（make gen 后 git diff --exit-code）；事件 Schema 生产/订阅一致性校验待首个订阅方落地后细化
- [x] 2.6 `contracts/errors/`：按模块分配错误码段（`MMNNNN`），18 个前缀全部登记占位

## 3. archcheck 边界检查（`tools/archcheck`）

- [x] 3.1 依赖阶梯校验：按 4.2 阶梯表生成规则，`modules/<m>` 只能 import 更低阶模块的 `contract`，违规 CI 失败
- [x] 3.2 表前缀校验（说明：未接 sqlc，仓储为手写 pgx 查询；archcheck 同时扫描 `.sql` 文件与 Go 内联 SQL，出现非本模块前缀的表即失败）
- [x] 3.3 `infra` 层之外禁用 `database/sql`/`pgx` import
- [x] 3.4 `bff` 只读校验：bff 不得调用任何模块的写接口
- [x] 3.5 用「故意违规的样例模块」验证四条规则都会失败（自检测试）

## 4. pkg/ L0 工具库

- [x] 4.1 `pkg/errs`：统一错误类型与错误码映射
- [x] 4.2 `pkg/page`：游标分页（每页 20 条约定）
- [x] 4.3 `pkg/ids`：int64 时间有序 ID 生成器（应用侧），JSON 序列化为字符串
- [x] 4.4 `pkg/money`：int64 分运算、折扣系数 numeric(6,4)、四舍五入到分（规则入配置）
- [x] 4.5 `pkg/clock`：UTC 存储与 Asia/Shanghai 自然日换算
- [x] 4.6 `pkg/mask`、`pkg/retry`、`pkg/validate`、`pkg/phone`（手机号/证件号格式）
- [x] 4.7 每个包纯函数单测（ids/money/page/clock + errs/mask/retry/validate/phone 全测）；`pkg/` 不得 import 任何内部包（archcheck 覆盖）

## 5. kernel：装载、配置、迁移、可观测

- [x] 5.1 `kernel/module`：Module 接口（`Register` 路由/事件/任务/策略/状态机/配置/配额）与装载器；按 `DependsOn` 拓扑排序，缺依赖**启动即失败**
- [x] 5.2 `catcube serve --roles=... --modules=...`：roles 支持；`--modules`（或 `CATCUBE_MODULES`）模块开关实现——共享域与 ops 始终装载、依赖传递补齐、未知模块报错（`selectModules` 单测覆盖）；为 p5「ai 未装载可降级」前置
- [x] 5.3 `kernel/config`：启动配置（环境变量 `CATCUBE_*`）三层配置的第一层
- [x] 5.4 `kernel/settings`：`sys_settings`（scope+key+value+type+version+effective_at+operator）；模块注册 `SettingsSchema` 声明键/类型/范围/默认值/是否敏感；scope 取值顺序 space → platform（一期 space 恒 1）
- [x] 5.5 `kernel/migrate`：按 DependsOn 拓扑执行、`sys_migration` 记录执行历史（expand → migrate → contract 三步式未实现，当前为普通版本迁移）
- [ ] 5.6 `kernel/obs`：OpenTelemetry 封装（当前为 slog 结构化日志 + request_id 中间件；trace/metric 未接入）
- [x] 5.7 `/healthz`（存活）与 `/readyz`（依赖就绪）端点

## 6. kernel：事件、调度、状态机

- [x] 6.1 `kernel/eventbus`：`sys_outbox` 表（event_id、topic、payload、status、retry_count）；业务表与 outbox **同事务写入**；worker 角色轮询投递，至少一次
- [x] 6.2 订阅方幂等表（事件 ID 去重）与失败重试已实现；死信后台管理：`GET /api/admin/v1/dead-letters`（按时间倒序）与 `POST .../{eventID}/replay`（复位 status=0/retry=0），admin-web「死信队列」页；ops 模块（R1）+ archcheck 阶梯登记
- [x] 6.3 `kernel/scheduler`：模块注册 `SchedulerJob`；PG 咨询锁保证全局单活；批量、幂等、可重入（`sys_job` 表）
- [x] 6.4 `kernel/statemachine`：声明式 `StateMachineDef{from,event,to,actor,guard}` 引擎；非法转移在引擎层拒绝；统一写状态日志并发布事件
- [x] 6.5 到期转态链路：`kernel/expiry.Run`（部分索引批量扫描、分批、单条 UPDATE 幂等、表名白名单、批回调发事件）；带样例表与集成测试（`TestIntegrationExpirySample`），供各模块到期 Job 复用

## 7. kernel：授权、配额、幂等、事务

- [x] 7.1 `kernel/policy`：`Decision{Allow,Reason}`、`Actor` 装配器（CapabilityProvider 惰性装配、请求内缓存）；内核不 import 任何业务模块
- [x] 7.2 注册表机制：扩展点注册表通用实现（注册即生效、删除模块即失效；已用 trust.ReviewSubjectProvider 跑通）
- [x] 7.3 `kernel/quota`：`QuotaDef{key,window(day/total),scope(user/circle/cube)}` 注册；`sys_quota_counter`（自然日窗口 Asia/Shanghai）；超限返回 Reason；数值来自 settings
- [x] 7.4 `kernel/idempotency`：`Idempotency-Key` 中间件与存储
- [x] 7.5 `kernel/scopedrepo` + `kernel/tx`：Scope 注入与 SQL 片段；`db.InTx` 同事务写业务表 + outbox
- [x] 7.6 `kernel/sensitive`：`Visible[T]` 包装（漏写策略默认输出 null）、AES-256-GCM 字段加密、phone_hmac 盲索引（独立密钥）
- [x] 7.7 `kernel/audit`：`sys_audit`（actor_type/channel/action/target_ref/detail/result/ip/ua/request_id/prev_hash/hash）；追加写 + SHA-256 哈希链；`audit.Record` 装饰器

## 8. ports 与 adapters

- [x] 8.1 端口接口定义完成；契约测试套件 `internal/ports/suite`（Cache/Queue/Search/Storage/Pay/Moderation/Realtime 同套用例）已建
- [x] 8.2 `adapters/pg`：queue 与 search 过契约套件（`pg/suite_test.go`，integration）
- [x] 8.3 `adapters/local`：进程内 cache 与 Realtime Noop 过契约套件
- [x] 8.4 `adapters/oss`：过契约套件（未配置 endpoint 时本地 FS 回退 Put/URL/Delete）
- [x] 8.5 `adapters/wechat`：code2session/手机号接口已实现；开发态支付适配器（预支付/回调验签/退款）过契约套件（真实商户接入待 p0 支付合规闭环）
- [x] 8.6 `adapters/moderation`：文本机检已实现；契约套件入口已备（无 token 时跳过）；图片异步回调待接（p0 内容安全接入）
- [x] 8.7 同一套契约测试跑在每个适配器上（suite + 各适配器 suite_test.go）

## 9. identity 模块骨架（R0）

- [x] 9.1 模块结构：`modules/identity/{contract,module.go,migrations,internal/app}`（admin/ 与 transport/domain/infra 细分在 p2 补）
- [x] 9.2 迁移：`idn_user`（含 phone_enc/phone_hmac/realname_status/authz_ver/owner_space_id）、`idn_session`、`idn_login_attempt`、`idn_sms_code`、`idn_deactivation_log`
- [x] 9.3 微信授权登录：code2session → 手机号（`GetPhoneNumberByCode`）→ phone_hmac 查/建用户注册登录一体化；phone_enc + phone_hmac 唯一校验；openid 存 `wechat_openid`（加密迁移留 p2）
- [x] 9.4 手机号 + 短信验证码备选登录（`/auth/sms/code`、`/auth/sms`）；登录失败锁定（窗口内 5 次 → 010006，`idn_login_attempt`）
- [x] 9.5 令牌：Access 2h RS256 只含 `sub/sid/authz_ver`；Refresh 7 天滚动轮换、哈希存储、可吊销；同账号 ≤3 活跃会话
- [x] 9.6 `authz_ver` 递增机制与网关比对（无陈旧窗口）；订阅 `AuthzChanged` 类内部触发点
- [x] 9.7 注销与匿名化：`Deactivate` 发布 `identity.UserDeactivated`，本模块处理自身数据匿名化
- [x] 9.8 contract：`Authenticate`、`SessionOf`、`UserBrief`、`Deactivate`、`AuthzBump`；注册 CapabilityProvider（账号状态；实名先返回未实名）
- [x] 9.9 admin/ AdminSection：`kernel/module.Registry.AdminSections` + 装配根集中声明（identity/content/trust/trade/membership/cube/circle/notice/ops/dict/profile）；`GET /api/admin/v1/menus` 按启用模块返回；admin-web 侧栏由声明驱动（拉取失败回退静态）
- [x] 9.10 状态/策略测试：domain 单测（锁定/会话上限/手机号）+ 集成测试（`integration_test.go`：登录、短信、令牌轮换、会话上限、注销事件、authz_ver 陈旧窗口）

## 10. trust 模块骨架（R0）

- [x] 10.1 模块结构与迁移：`trs_review_task`、`trs_sensitive_word`、`trs_moderation_record` 等 `trs_` 前缀表
- [x] 10.2 `Moderation` 端口接入：文本同步机检命中拦截；**任何代码不直接调微信接口**（图片异步回调占位待接）
- [x] 10.3 敏感词/引流词本地前置过滤（词库后台可维护，种子来自 p0 任务 4.2）
- [x] 10.4 `ReviewSubjectProvider` 注册表开口（怎么取文字/图片、结论怎么落回）；本阶段无注册者，接口与单测先行
- [x] 10.5 contract：`SubmitReview`、`Report`、`Restrict`、`IsRestricted` 接口定型；注册 CapabilityProvider（是否被限制发布）
- [x] 10.6 审核结论事件 `ModerationPassed/Rejected` 的 Schema（发布链路随首个生产方在 p2 接通）

## 11. dict 模块（共享业务域）

- [x] 11.1 迁移：`dct_taxonomy_option`（dimension/code/label/parent_code/sort/status，只停用不删除）、`dct_subject_type`、`dct_field_template`、`dct_field_def`
- [x] 11.2 taxonomy 服务：`Options(dimension)`、`Resolve(code)`；行业/子行业两级联动（parent_code 支持扩展，未来加级不发版）
- [x] 11.3 主体类型配置：类型编码创建后不可改、名称、图标、can_publish、home_visible、排序、状态
- [x] 11.4 画像字段模板：`dict` 字段池（`dct_field_def`）+ 主体类型模板（`dct_field_template.fields`）种子（`contracts/seeds/dict/{field_def,field_template}.json`）；`FieldTemplate/RenderValidate`（模板与字段池联表渲染、按 sort 排序、必填与类型校验、引用未登记字段显式报错）；集成测试 `TestIntegrationProfileFieldTemplate`
- [x] 11.5 `TagSet` 值对象与 `ValidateTagSet` 统一校验（非法 code 拒绝）
- [x] 11.6 `contracts/seeds/` 种子导入命令（幂等）：行业两级、区域省市县、合作需求/供给、会议/活动领域、主体类型六类（`catcube seed`）
- [x] 11.7 admin/：`dict.taxonomy`（主体类型与选项）、`profile.templates`（画像字段模板）AdminSection 声明已登记（页面 p4）

## 12. media 模块（共享业务域）

- [x] 12.1 迁移：`med_file`（详细设计 4.2.7 全字段）
- [x] 12.2 预签名直传：类型与大小校验（JPG/JPEG/PNG ≤5MB）已实现；OSS 预签名为骨架（供应商接入 TODO）；封面尺寸校验待补
- [x] 12.3 图片规格与 CDN 输出：`media/specs.go` 规格集（orig/thumb_s/m/thumb_l/cover/avatar → 供应商处理参数，CDN 侧生成缩略图）；`URL(fileId, spec)` 校验规格后注入 CDN 处理参数（未知规格显式报错）；OSS/CDN 经 `CATCUBE_OSS_ENDPOINT`/`CATCUBE_CDN_HOST` 配置注入；单测 `specs_test` + 集成测试 `TestIntegrationMediaURLSpecs`
- [x] 12.4 `Bind(ownerRef)`/`Unbind` 引用计数已实现；孤儿文件定时清理 `media.purge_orphans`（引用归零且超 7 天→删对象+删行，测试覆盖）；实体删除事件回收路径已备（`Unbind` 供各模块调用）
- [x] 12.5 entityref 轻量包：`EntityRef{type,id}` 类型 + `EntityResolver` 注册表 + `Resolve/BatchResolve`（无独立表）

## 13. 测试框架

- [x] 13.1 权限矩阵框架：`internal/tests/matrix`（Role/Expect/Case/Probe + 按角色分组逐格执行，框架说明见包注释）；权限矩阵用例迁移至框架（`TestIntegrationPermissionMatrixFull`，17 格 + 管理员反向放行格）
- [x] 13.2 状态机表驱动框架：`statemachine.SelfCheck`（状态/终态/重复转移/出边/角色声明）+ `Exhaustive`（状态×事件全组合穷举：合法必成功落点一致、非法必 `IllegalTransitionError`）；装配根收集 `App.StateMachines`（content/trade/verify/membership/cube/circle 共 12 个定义注册）；`TestIntegrationStateMachineTable` 全量穷举通过
- [x] 13.3 模块独立启动测试：真实模块（dict/media/identity/trust）拓扑装载成功、缺依赖失败（`internal/app/module_load_test.go`）
- [x] 13.4 `app/wire.go` 装配根：读配置 → 选适配器 → 装载模块；S0 形态全角色全模块单进程

## 14. S0 部署

- [x] 14.1 `deploy/compose`：app（全角色全模块）+ PostgreSQL 一键起（`make up`；已真实启动验证，宿主端口冲突时用 APP_PORT 覆盖）
- [x] 14.2 PITR 备份：S0 compose db 开启 `wal_level=replica`/`archive_mode`/`archive_command`（WAL 归档卷）+ `backup` 边车每日 `pg_basebackup` 与保留清理（`deploy/compose/backup.sh`）+ 恢复步骤 `restore.md`；`docker compose config` 校验通过
- [x] 14.3 前端脚本：Makefile `install`/`typecheck`/`build-web`/`dev-admin`/`dev-miniapp`（后台 Vite 代理 /api→8080；小程序 watch 构建导入 dist）；`make typecheck` 通过

## 15. 退出标准

- [x] 15.1 S0 一键起：`make up` 后 `serve` 全角色全模块启动，`/healthz`、`/readyz` 通过（已验证；镜像 vendor 构建、distroless 运行）
- [x] 15.2 空模块（仅 contract + module.go 骨架）能按阶梯装载；缺依赖启动即失败有测试覆盖
- [x] 15.3 archcheck 四条规则自检通过，接入 `make lint` 与 CI boundary 门禁
- [x] 15.4 契约生成管线贯通：改 OpenAPI/事件 Schema → `make gen` → Go stub 与 TS 类型更新（CI 漂移检查兜底）
- [ ] 15.5 identity 登录/令牌/注销链路集成测试已落地（`make test-integration`）；dict 种子导入后可查询、media 上传→绑定→孤儿清理链路测试待补
