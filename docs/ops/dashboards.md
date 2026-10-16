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

## 按模块拆分

`obs` 日志与指标统一带 `module` 维度（日志字段 `module`，指标建议后续加 `module` 标签）以便判断哪个模块需要独立拆分（技术方案 11.5）。

## 健康与容量

| 项 | 指标 | 口径 |
| --- | --- | --- |
| 可用性 | 负载侧 5xx 比例、`/readyz` 探测 | ≥99.5% |
| 首屏 | 客户端 `bff /feed`、`/aggregate/*` 时长 | P95 ≤3s（压测场景 p6 §2） |
| 常规查询 | 服务端 API 时延 | P99 ≤1s（后调为 ≤3s 告警，见 alerts.md） |
| 链路追踪 | `trace_id`/`span_id`（W3C Trace Context） | 日志与事件统一带 `trace_id`：请求头 `Traceparent` 透传，异步事件经 Outbox 续接，可端到端串联「请求 → 事件 → 订阅处理」 |
