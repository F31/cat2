# 告警清单与初始阈值（p6 §5.4）

阈值均为**初始值可配**（环境变量）；应用层告警经 `obs.Metrics.Alert` 记数并打点日志，
指标经 `GET /metrics` 暴露，由监控栈抓取并转发到值班渠道。基础设施类告警（P99/错误率/DB/Redis）由负载与监控栈产生，应用侧提供指标支撑。

## 应用层告警（已实现）

| 告警名 | 触发条件 | 阈值（env，默认） | 指标 | 处置动作 |
| --- | --- | --- | --- | --- |
| `outbox_backlog_high` | Outbox 积压超阈 | `CATCUBE_ALERT_OUTBOX_BACKLOG`（1000） | `catcube_outbox_backlog` | 查 worker 是否存活/订阅处理是否报错；必要时扩容 worker |
| `outbox_oldest_age_high` | 最老事件年龄超阈 | `CATCUBE_ALERT_OUTBOX_OLDEST_SECONDS`（300） | `catcube_outbox_oldest_age_seconds` | 同上；确认无长事务阻塞投递 |
| `outbox_dead_high` | 死信超阈 | `CATCUBE_ALERT_OUTBOX_DEAD`（100） | `catcube_outbox_dead` | 后台「死信队列」排查原因后重放 |
| `fulfillment_retry_exceeded` | 履约重试超阈/死信 | `CATCUBE_ALERT_FULFILL_RETRY`（5） | `catcube_fulfillment_stuck` | 后台「履约补偿」重试；定位未注册履约方/接口故障 |
| `review_sla_overdue` | 审核任务超 SLA | 固定（SLA 到期判定） | `catcube_review_sla_overdue` | 增派审核人力/检查队列积压原因 |
| `review_backlog_high` | 审核积压超阈 | `CATCUBE_ALERT_REVIEW_BACKLOG`（200） | `catcube_review_backlog` | 同上 |
| `audit_chain_broken` | 审计链断链/篡改 | 固定（校验失败即告警） | `catcube_audit_chain_ok`=0 | **安全事件**：立即冻结后台写操作、保全证据、按事故流程上报 |
| `contact_scrape_suspected` | 联系方式查看超日限频 | `CATCUBE_CONTACT_DAILY_LIMIT`（200） | `catcube_contact_reveal_total` | 复核该账号行为；必要时限制/封禁并调整限频 |

## 基础设施/接入层告警（监控栈配置）

| 告警项 | 触发条件（初始值） | 数据来源 | 处置动作 |
| --- | --- | --- | --- |
| API P99 时延 | P99 > 3s（持续 5m） | 负载/接入层（Nginx/网关）+ 服务端时延 | 查慢查询/SQL 索引、缓存命中；必要时限流 |
| 错误率 | 5xx 比例 > 1%（持续 5m） | 负载/接入层 | 看日志 `request_id` 串链定位模块 |
| 支付回调失败 | `catcube_pay_notify_failed_total` 增量 > 0 | 指标 | 核对验签配置/金额；确认非攻击后排查渠道 |
| DB 主从延迟 | 复制延迟 > 10s | PG（`pg_stat_replication`） | 查长事务/大事务；评估读流量切换主库 |
| Redis 不可用 | 探测失败 | 监控栈 | 已内置降级（回退进程内缓存，见 `adapters/redis.Fallback`）；恢复后自动切回 |
| 磁盘/备份 | 备份失败或 WAL 归档中断 | 备份边车/`archive_command` 日志 | 立即排查，PITR 断档需重做全备并验证 |
| 审核 SLA 将超时 | 距 SLA 到期 < 30m | `catcube_review_sla_overdue` + 队列 | 值班审核跟进 |

## AI 相关（p5 §8.2；已落地告警见下）

| 告警 | 触发 | 处置 |
| --- | --- | --- |
| `ai_adopt_rate_drop` | 采纳率 < `ai.alert.adopt_rate.min`（默认 30%）且样本 ≥ `ai.alert.adopt.min_sample`（默认 20） | 检查提示词/模型版本；必要时关开关回退 |
| `ai_cost_anomaly` | 当日成本 > `ai.alert.cost.fen.daily`（分，0=不告警） | 复核用量；必要时限流/降级为规则路径 |

> 由定时任务 `ai.cost_guard`（每 10 分钟）评估，阈值经后台 `GET/PUT /api/admin/v1/settings` 热生效；
> 指标 `catcube_ai_cost_fen_daily`/`catcube_ai_adopt_rate_pct`/`catcube_ai_tokens_daily`（`GET /metrics`）。

| 指标类告警（预留） | 触发 | 处置 |
| --- | --- | --- |
| AI 降级升高 | `catcube_ai_degraded_total` 日环比 +50% | 检查上游可用性与开关配置（关闭开关即全量回退原表单） |
| AI 幻觉升高 | `catcube_ai_output_dropped_total` 异常增长 | 复核提示词版本（`ai_prompt`，未评测版本不生效） |

## 处置通用要求

1. 告警须关联 `request_id` 与 `module`，便于定位（日志已统一结构化与脱敏，`logscan` 保证敏感字段不入日志）。
2. 每个告警定义**回滚动作**：开关类（特性开关关闭）、队列类（后台重放/补偿）、配置类（阈值调整）。
3. 安全类告警（`audit_chain_broken`、`contact_scrape_suspected`）须在事故流程中留痕并复盘。
