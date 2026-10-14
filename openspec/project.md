# 猫平方商业一体化综合服务平台 · 项目上下文

## 项目目的

猫平方是面向商务合作的综合服务平台。一期交付「两端 + 一个移动端身份」：

- **微信小程序**（用户端）：广场（匹配卡片流/搜索）、圈子、猫立方（协作工作台）、消息、我的
- **总管理后台**（Web 运营端）：审核、内容、认证、订单结算、配置中心等
- **运营商**：小程序内身份，不设独立后台

一期主场景：「找对人 + 联系上」（注册 → 画像 → 匹配/搜索 → 详情 → 联系 → 付费开通会员）。猫立方核心链路（建立方、项目、任务、报备、订阅分摊）须可跑通。AI 能力为可选装载、可降级的增强。

## 权威文档

- 《产品需求说明书 v2.5》：一期**唯一需求依据**，冲突时以它为准
- `docs/猫平方_技术方案设计_v2.5.md`：架构基线
- `docs/猫平方详细设计方案_v1.4.docx`：详细设计（数据建模、UI、API、流程）
- 《猫平方6方合作服务平台需求概要》：战略方向参考，只产生架构预留项，不扩一期范围

## 技术栈

- 后端：Go（标准库 HTTP / chi），单一 go.mod、单一二进制（模块化单体）
- 数据访问：pgx + sqlc（显式 SQL，CI 校验表前缀）
- 数据库：PostgreSQL（必选，一期检索用 pg_trgm/中文分词；pgvector 可选二期）
- 对象存储：云 OSS（必选）；Redis（可选，S1 引入）；队列一期用 PG 任务表（Outbox）
- 小程序：Taro + React + TypeScript（立项两周内原型验证，不通过回退原生）
- 后台：React + Ant Design
- 前端工程：pnpm workspace monorepo
- 契约：OpenAPI 3（HTTP）+ 事件 JSON Schema，契约先行、生成代码
- 可观测：OpenTelemetry（trace/metric/log 统一入口，带 module 标签）
- 不引入：Consul/Nacos/Kong/Argo 等重型中间件

## 架构约定（所有变更必须遵守）

### 模块与依赖阶梯

17 个模块（15 业务模块 + 共享域 dict/media + 轻量包 entityref），是代码包边界而非服务边界。**模块只能依赖编号更低的阶**（同阶也不行）：

| 阶 | 模块 |
| --- | --- |
| shared | dict、media、entityref（不得依赖任何业务模块） |
| R0 | identity、trust、ai |
| R1 | profile、msg、trade、social |
| R2 | verify、content |
| R3 | membership、cube |
| R4 | circle、channel |
| R5 | discovery（只依赖低阶的**事件类型**） |
| R6 | notice（只依赖低阶的**事件类型**） |

向上通信只有两种方式：**领域事件**（低阶发布、高阶订阅）与**注册表**（低阶开口 interface、高阶启动时注册实现，共 9 类扩展点）。跨模块读聚合只放 `bff`。跨模块写操作用「本地事务 + Outbox 事件 + 订阅方幂等」最终一致；强一致（如分摊成团）限定在同一模块内。

### 边界强制

- 模块实现放 `modules/<m>/internal/`，对外只有 `contract/`（Facade + DTO、事件类型、注册表接口），Go 编译器强制
- CI `tools/archcheck`：依赖阶梯、sqlc 表前缀（一表一 owner）、infra 之外禁用 database/sql/pgx、bff 只读
- `CODEOWNERS`：contract/ 变更须模块 owner 审批
- 运行时：ScopedRepo 强制注入 Scope；敏感字段 `Visible[T]` 默认不输出

### 数据约定

- 表前缀即归属：`idn_/trs_/ai_/dct_/med_/prf_/msg_/trd_/soc_/vfy_/cnt_/mbr_/cub_/cir_/chn_/dsc_/ntc_/sys_`
- 跨模块不建外键，只存对方 ID（EntityRef{type,id}）
- 主键 int64 时间有序（应用侧生成），JSON 一律输出字符串；金额 int64 分；时间 timestamptz(UTC)，自然日按 Asia/Shanghai
- 通用列：id、created_at、updated_at、version（乐观锁）、status、closed_at；**不物理删除**
- 多站点预留：所有请求带 space_id（一期恒 1）；`prf_org/cnt_item/cir_circle/cub_cube/sys_settings` 与后台账号带 `owner_space_id`；内容类带 `visibility`
- 迁移：每模块 `migrations/`，按 DependsOn 拓扑执行，expand → migrate → contract

### 规则即数据

价格、比例、限额、权重、字典选项、主体类型全部配置化：`SettingsSchema` / `QuotaDef` / taxonomy，后台可配、不发版、带版本与审计。PRD 11.5 的限额逐项落地为 `QuotaDef`。

### 状态机显式化

PRD 11.1 的 17 张状态流转表每张归属一个模块，以 `StateMachineDef{from,event,to,actor,guard}` 声明，内核引擎执行，非法转移在引擎层拒绝；自动动作只做「到期转态」，不做「自动通过」。

## 命名约定

- 模块名小写英文，与表前缀、目录、事件前缀、错误码段、前端 `features/` 一致
- 事件：`module.PastTense.vN`，载荷含事件 ID/occurred_at/space_id，**不含敏感字段**
- API：`/api/{mp|admin|open}/v1/<module>/<resource>`，资源名复数，动作用子资源
- 错误码：`MMNNNN`（前两位模块号），在 `contracts/errors/` 登记
- 提交：Conventional Commits，scope = 模块名

## 测试策略（质量门禁）

| 类型 | 位置 |
| --- | --- |
| 边界检查（archcheck） | CI 必过 |
| 权限矩阵（PRD 3.2/3.3 每格一条表驱动用例） | `tests/matrix` |
| 状态机（17 张表合法/非法转移） | 各模块 domain + `tests/` |
| 支付与分摊（回调重复/乱序、并发成团、退款幂等、对账差异） | `tests/trade` |
| 契约（OpenAPI/事件 Schema 生产消费一致） | CI |
| 端口契约套件（同一套测试跑每个适配器） | `ports/*/suite` |
| 模块独立启动（证明可拆） | CI 矩阵 |
| 压测（PRD 10.4.1 目标 + 5~10 倍余量） | `tests/perf` |

## 工程结构（monorepo 目标态）

```
catcube/
├── apps/server/     # Go 后端：cmd/ pkg/ internal/{kernel,ports,adapters,shared,modules,bff,app,gen}
├── apps/miniapp/    # 小程序：shell/ features/<module>/ shared/ platform/ ai-entry/
├── apps/admin-web/  # 后台：shell/ features/<module>/ shared/ governance/
├── packages/        # api-client domain-types policy-hints taxonomy-kit form-kit utils
├── contracts/       # openapi/ events/ errors/ seeds/
├── deploy/ tools/ tests/ docs/{adr,modules,prd-trace}/
└── CODEOWNERS Makefile
```

## 关键红线

- 支付合规：平台统一收款，禁个人对个人、二级清算、多级分销（schema 上 `referrer_operator_id` 单列无上级字段）；用户端无退款入口，退款只由后台财务发起
- 内容安全：任何代码不直接调微信内容安全接口，统一经 `Moderation` 端口；命中不放行
- 敏感数据：手机号/证件号/联系方式 AES-256-GCM + phone_hmac 盲索引；联系方式查看全量审计 + 限频告警
- 后台敏感操作（退款/封禁/改价/导出/结算）二次确认 + 审计；复核与执行不得同一人（内核策略强制）
