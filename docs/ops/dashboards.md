# 业务看板口径（p6 §5.3）

数据来源：服务端 `kernel/obs` 指标表，经 `GET /metrics`（Prometheus 文本格式）暴露；
生产由 S1 监控栈（Prometheus + Grafana 或云监控）抓取。指标名稳定，口径变更须同步本文件。

> 说明：本版本为**服务端口径**（无客户端埋点时也能看漏斗主链路）；产品埋点（小程序上报）后续接入不得与本口径冲突。

| 看板项 | 指标 | 类型 | 口径 | 埋点位置 |
| --- | --- | --- | --- | --- |
| 注册 | `catcube_user_registered_total` | counter | 微信登录首次建号（一次登录只记一次新号） | `identity` 建号事务内 |
| 登录 | `catcube_user_login_total`* | counter | 登录成功次数 | `identity`（预留，随 OTel 落地补） |
| 画像完成度 | `catcube_profile_card_set_total`* | counter | 名片保存成功（第一层）；第二层完成以 profile 完成度为维度 | `profile`（预留） |
| 匹配点击 | `catcube_match_feed_total` | counter | feed 返回的匹配条数（曝光代理） | `discovery` `/matches` |
| 匹配点击 | `catcube_match_click_total` | counter | 查看匹配解释（点击代理） | `discovery` `/matches/{ref}/explain` |
| 联系发起 | `catcube_contact_reveal_total` | counter | 成功下发联系方式次数 | `bff` `/aggregate/contact` |
| 联系发起 | `catcube_session_ensure_total` | counter | 会话创建/确保次数（含单聊/群聊） | `msg` `/sessions` |
| 会员转化 | `catcube_member_opened_total` | counter | 会员权益开通成功 | `membership.Fulfill` |
| 猫立方开通 | `catcube_cube_opened_total` | counter | 立方订阅履约成功 | `cube.Fulfill`（`cube_plan`） |
| 支付成功率 | `catcube_order_paid_total` | counter | 支付回调置为已支付次数（去重，不含重复回调） | `trade` 回调审计 |
| 支付成功率 | `catcube_pay_notify_failed_total` | counter | 回调验签/金额校验失败次数 | `trade` 回调审计 |
| 审核积压 | `catcube_review_backlog` | gauge | 待审任务数（status=0） | `trust.sla_monitor`（1 分钟） |
| 审核 SLA | `catcube_review_sla_overdue` | gauge | 超 SLA 待审数 | `trust.sla_monitor` |
| Outbox 积压 | `catcube_outbox_backlog` | gauge | 未投递事件数（status 0/1/2） | `kernel.outbox_monitor`（30s） |
| Outbox 最老事件 | `catcube_outbox_oldest_age_seconds` | gauge | 最老未投递事件年龄（秒） | 同上 |
| Outbox 死信 | `catcube_outbox_dead` | gauge | status=9 事件数 | 同上 |
| 审计链完整性 | `catcube_audit_chain_ok` | gauge | 1=近窗链条校验通过，0=断链/篡改 | `kernel.audit_verify`（1 小时） |
| 履约兜底 | `catcube_fulfillment_stuck` | gauge | 重试超阈/死信履约单数（付了钱没开通） | `trade.fulfill`（30s） |
| 事件投递 | `catcube_outbox_relayed_total` / `catcube_outbox_retry_total` | counter | 成功投递/重试次数 | `eventbus.Relay` |
| AI 调用 | `catcube_ai_call_total` | counter | AI 调用次数（含降级尝试） | `ai` 模块 |
| AI 降级 | `catcube_ai_degraded_total` | counter | 开关关闭/上游失败/解析失败降级次数 | `ai` 模块 |
| AI 采纳率 | `catcube_ai_adopted_total` / `catcube_ai_rejected_total` | counter | 采纳/放弃反馈（采纳率 = 采纳/(采纳+放弃)） | `ai.Feedback` |
| AI 成本 | `catcube_ai_cost_fen_total` / `catcube_ai_tokens_total` | counter | 成本（分，按 `ai.cost.fen_per_1ktoken` 估算）与 token 量 | `ai` 模块 |
| AI 配额 | `catcube_ai_quota_exceeded_total` | counter | 日额度/预算用尽次数 | `ai.checkQuota` |
| AI 隐私 | `catcube_ai_privacy_scrub_total` | counter | 提示词中剥离的敏感信息条数（应持续为 0 表示上游无泄漏面） | `ai.sanitizeForPrompt` |
| 审核辅助 | `ai_call_log.feature='review'` | 记录 | 审核建议调用与耗时（`catcube_ai_*` 同口径） | `ai.ReviewSuggestion` |
| AI 输出收敛 | `catcube_ai_output_dropped_total` | counter | 模型幻觉字段被丢弃数（异常升高说明提示词需调优） | `ai` 模块 |

`*` 为预留指标（口径已定，随 p5/OTel 或后续增量补齐）；其余均已实现并有测试覆盖（见 `TestIntegrationBusinessMetrics`）。

## 漏斗（注册 → 画像 → 匹配点击 → 联系发起 → 付费）

```
注册(catcube_user_registered_total)
  → 画像(profile 完成度)
  → 匹配点击(catcube_match_click_total / catcube_match_feed_total)
  → 联系发起(catcube_contact_reveal_total / catcube_session_ensure_total)
  → 付费(catcube_order_paid_total → catcube_member_opened_total / catcube_cube_opened_total)
```

## 平台看板接口（p4 §7.1）

`GET /api/admin/v1/dashboard` 返回实时指标（不在业务表写统计）：用户总数/今日新增/已实名、内容展示中、今日搜索与匹配点击（埋点）、今日联系发起（埋点）、在期会员、今日支付单与 GMV、活跃圈/立方、审核积压与超 SLA、死信、AI 调用与降级。

## 看板分组与漏斗（p4 §7.3）

- 分组：`platform`（用户/内容）、`collaboration`（圈/需求/立方/项目/任务/分摊）、`funds`（今日支付与 GMV、退款笔数与金额、履约异常）、`security`（24h denied 与登录锁定、审核积压与超 SLA、联系方式抓取告警）。
- 漏斗：`GET /api/admin/v1/dashboard/funnel?days=N`，步骤 注册→画像完成→匹配点击→联系发起→支付成功（去重用户 + 步间转化率；支付另给业务表兜底口径）。
- 未覆盖：渠道/运营商看板（依赖 channel 模块与 #13）。

## 埋点（p4 §7.2）

- 事件登记：`contracts/telemetry/README.md`（8 个漏斗事件；属性白名单）。
- 存储：`tlm_event`（独立前缀，不污染业务表）；上报 `POST /api/mp/v1/telemetry`（≤50 条/次）。
- 服务端二次过滤：未登记事件丢弃、props 仅白名单键、值含手机号/证件号形态剔除。

## 按模块拆分

`obs` 日志与指标统一带 `module` 维度（日志字段 `module`；OTel 指标镜像带 `module` 标签，见下节）以便判断哪个模块需要独立拆分（技术方案 11.5）。

## 健康与容量

| 项 | 指标 | 口径 |
| --- | --- | --- |
| 可用性 | 负载侧 5xx 比例、`/readyz` 探测 | ≥99.5% |
| 首屏 | 客户端 `bff /feed`、`/aggregate/*` 时长 | P95 ≤3s（压测场景 p6 §2） |
| 常规查询 | 服务端 API 时延 | P99 ≤1s（后调为 ≤3s 告警，见 alerts.md） |
| 链路追踪 | `trace_id`/`span_id`（W3C Trace Context） | 日志与事件统一带 `trace_id`：请求头 `Traceparent` 透传，异步事件经 Outbox 续接，可端到端串联「请求 → 事件 → 订阅处理」 |

## OpenTelemetry 接入（p1 §5.6 / p6 §5.1）

服务端已内建 OTel SDK 接入点，**默认关闭**（与业务同口径：确认后再开），关闭时行为与既有轻量 trace/`/metrics` 完全一致。

- 代码：`internal/adapters/otelx`（`Setup`：组装 OTLP gRPC Trace/Metric Exporter + Batch/Periodic 处理器 + W3C 传播器；`SetupLogs`：OTLP 日志导出 + slog 桥接），`kernel/obs`（HTTP 中间件起 server span，续接上游 `traceparent`；`obs.SpanFromContext` 供业务补录属性；`obs.NewTeeHandler` 控制台 + OTel 双出口；`obs.RegisterOTelMetrics` 指标镜像）。
- **统一入口（p6 §5.1）**：启用后 trace / metric / log 三路同源出口到 OTLP。
  - 日志：`slog` 记录经 `otelx.NewLogHandler` 转为 OTel 日志记录，与控制台 JSON 并存（`obs.NewTeeHandler`），**脱敏在最外层**（`NewRedactingHandler` 先于扇出，OTLP 不含敏感信息）；使用 `InfoContext`/`obs.Log(ctx)` 时按 ctx 关联 `trace_id`/`span_id`。
  - 指标：`obs.RegisterOTelMetrics` 将内置 `/metrics` 注册表镜像为 OTel 异步 Gauge（`catcube.metric.snapshot`），每条观测带 `module` 标签（由指标名前缀推导，未命中归 `core`）。
- 环境变量：
  | 变量 | 默认 | 说明 |
  | --- | --- | --- |
  | `CATCUBE_OTEL_ENABLED` | `false` | 总开关；关闭时全局保持 no-op |
  | `CATCUBE_OTEL_ENDPOINT` | `otel-collector:4317` | OTLP gRPC 端点 |
  | `CATCUBE_OTEL_SERVICE` | `catcube` | `service.name`（S1 中 api/worker 各设） |
  | `CATCUBE_OTEL_INSECURE` | `true` | 集群内明文 gRPC |
  | `CATCUBE_OTEL_SAMPLE_RATIO` | `1` | 采样比例（`ParentBased(TraceIDRatioBased)`，越界收敛为 1） |
- 部署：`deploy/compose/otel-collector.yaml` + S1 编排 `otel-collector` 服务（`profiles: ["observability"]`，可选启停）。启用：
  ```
  CATCUBE_OTEL_ENABLED=true docker compose -f deploy/compose/docker-compose.s1.yml --profile observability up -d
  ```
  Collector 接收 OTLP（4317/4318），`debug` 输出到 stdout，并在 `:8889` 以 Prometheus 格式暴露指标供监控栈抓取。接入托管后端（Tempo/Jaeger/云 APM）时在 `otel-collector.yaml` 增加对应 exporter 并加入 pipelines。
- 降级：导出端点不可达时 Exporter 异步失败且被静默（`otel.SetErrorHandler`），业务与日志不受影响；请求级 `trace_id` 仍由中间件生成/透传。

测试：`internal/kernel/obs/obs_trace_test.go`（装配 SDK 时导出 server span 且父 span 续接上游；未装配时回退轻量生成）、`internal/adapters/otelx/otelx_test.go`（关闭 no-op / 启用构建与默认值）、`otelx/logs_test.go`（slog→OTel 桥接：级别/正文/组属性展平、关闭返回 nil）、`kernel/obs/otel_metrics_test.go`（指标镜像带 `module` 标签）、`kernel/obs/tee_test.go`（扇出前先脱敏）。

