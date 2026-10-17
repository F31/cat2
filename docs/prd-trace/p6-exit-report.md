# p6 退出状态报告（截至 2026-10-17）

> 与 `docs/ops/launch-checklist.md`（逐项检查单）配套：本报告按 p6 章节给出**状态、证据、剩余阻塞**。
> 图例：`✅ 完成` / `◐ 部分（已具备代码或本地验证，待环境/产品）` / `⛔ 阻塞`。

## 汇总

| 章节 | 状态 | 说明 |
| --- | --- | --- |
| 1 安全 | ◐ | 越权/支付回放/审计断链/日志脱敏/CI 门禁 ✅；渗透与抓取压测待环境 |
| 2 性能与容量 | ◐ | 造数与压测工具 ✅ 且本地并发 100 全达标；S1 正式复跑待环境 |
| 3 部署与可靠性 | ◐ | S0/S1 编排、健康检查与优雅停机、PITR 配置与演练 ✅；云环境 PITR 演练待执行 |
| 4 降级矩阵 | ✅ | 搜索/内容安全/notice/Redis/履约/AI 六类降级均已自动化验证 |
| 5 可观测 | ◐ | 结构化日志+脱敏+trace 贯通+`/metrics`+看板口径+告警清单+OTel trace/metric/log 统一入口与指标 `module` 标签 ✅；监控栈后端接入待环境 |
| 6 灰度发布 | ◐ | 特性开关（用户/比例/白名单）✅；小程序审核发版与灰度批次执行属外部 |
| 7 验收与追溯 | ◐ | PRD 11.4 追溯矩阵、待确认复核、上线检查单 ✅；p4 完成使运营商/渠道/RBAC 条目转 ✅；剩余 3 项产品/发版口径；签署待外部 |
| p4（全量） | ✅ | 渠道/运营商、RBAC 五角色、审计体系、认证全类型、后台页面、配置中心自动渲染、导出/看板/埋点全部完成；`p4-channel-admin` 59/59 |
| 8 退出标准 | ◐ | p4 阻塞已解除；8.1 剩余产品/发版口径 3 项；8.2/8.3 待 S1/监控/灰度等外部环境 |

## 1. 安全

- ✅ 权限矩阵自动化（游客/未实名/非会员/非管理员 × 关键资源）：`TestIntegrationPermissionMatrixFull`（框架 `internal/tests/matrix`）
- ✅ 后台越权修复：`Route.Admin` 强制 `requireAdmin`（非管理员 403）；`idn_user.admin_role` 过渡实现 + `catcube grant-admin`
- ✅ 支付回放/伪造/乱序：拒绝并全量审计（`TestIntegrationPayReplayAndForgery`）
- ✅ 审计哈希链增量断链/篡改检测与告警（`kernel.audit_verify`，修复 `char(64)` 补空格取链偏差）
- ✅ 日志脱敏与扫描：`obs.RedactingHandler` + `tools/logscan`（`make lint` 必过）
- ✅ CI 安全门禁：govulncheck（依赖漏洞）+ gitleaks（密钥扫描）
- ◐ 联系方式抓取：限频 + 告警已实现（`CATCUBE_CONTACT_DAILY_LIMIT`，`TestIntegrationContactScrapeRateLimit`）；批量压测待环境
- ⛔ 渗透测试（OWASP Top 10）：外部执行

## 2. 性能与容量

- ✅ 造数：`tests/perf/seed.sh`（1 万用户/内容、1 千圈、100 立方；幂等；独立压测库）
- ✅ 压测工具：`tools/loadtest`（无外部依赖；P50/P95/P99、RPS、状态码构成；门槛不达标退出码非 0）
- ✅ 本地基线（并发 100）：feed P95 71ms（≤3s）、search P99 30ms（≤1s）、detail P99 777ms（≤1s），错误率 0%
  → `tests/perf/reports/2026-10-16-local.md`
- ✅ Outbox 吞吐：批量幂等/确认后 **283 → 18,483 条/秒**（积压 5000 条零丢失零死信，`TestIntegrationOutboxBacklogDrain`）
- ◐ S1 形态正式复跑（api×2 + PG 主从 + Redis）与 5~10 倍余量验证：待独立环境

## 3. 部署与可靠性

- ✅ S0 `make up`；S1 编排（api×2 + worker/scheduler + PG 主从流复制 + Redis + 备份边车）
- ✅ 健康检查（`/healthz`、`/readyz`，容器探针 `catcube probe`）与**优雅停机**（`CATCUBE_DRAIN_SECONDS`；先摘流量→等在途→退出；`ready` atomic）
  → 演练 `deploy/scripts/drain-drill.sh` 实测 PASS（129 请求 0×5xx、80×503、5s 退出）
- ✅ PITR：WAL 归档 + 每日全备 + 保留清理；恢复步骤 `deploy/compose/restore.md`
  → 演练 `deploy/scripts/pitr-drill.sh` 实测 PASS（时间点恢复精确到基线，RTO 111s）
- ◐ 云环境 PITR 演练与 RPO≤5min/RTO≤1h 记录：待云托管库执行

## 4. 降级矩阵（全部自动化验证）

| 场景 | 行为 | 证据 |
| --- | --- | --- |
| 搜索不可用 | pg_trgm 失败 → ILIKE 简单检索 | `TestSearchSimpleFallback` |
| 内容安全超时 | 保持「审核中」+ 机检重试队列，不放行 | `TestIntegrationModerationTimeoutGate` |
| 通知故障 | 业务不受影响、事件积压、恢复后重放补齐无丢失 | `TestIntegrationNoticeFailureDoesNotBlockBusiness` |
| Redis 不可用 | 回退进程内缓存（主备缓存） | `adapters/redis` 单测（`TestFallbackDegrade`） |
| 履约异常 | 重试超阈/死信告警 + 后台手动补偿 | `TestIntegrationFulfillmentAlertAndManualCompensation` |
| AI 不可用 | 关闭开关/上游故障 → `030001`，回退原表单 | `TestIntegrationAIFlagGateAndDegrade` |

## 5. 可观测

- ✅ 日志：结构化 + `request_id`/`module`/`user_id`（脱敏）+ `trace_id`/`span_id`
- ✅ trace 贯通：W3C `traceparent` 透传/生成；事件与 Outbox 携带 trace，订阅方上下文续接（`TestIntegrationTracePropagation`）
- ✅ 指标：`GET /metrics`（Prometheus 文本）；看板口径 `docs/ops/dashboards.md`（注册/画像/匹配点击/联系发起/会员转化/支付成功率/猫立方开通/审核积压/Outbox/审计链/履约/AI）
- ✅ 告警：应用层 8 类 + 接入层清单与阈值 `docs/ops/alerts.md`；`TestIntegrationOutboxBacklogAlert`、`TestIntegrationAuditChainBrokenAlert` 等
- ✅ OTel 全链路入口：`otelx.Setup`（Trace/Metric）+ `SetupLogs`（Log，slog 桥接）+ `obs.RegisterOTelMetrics`（`/metrics` 镜像为 OTel Gauge，带 `module` 标签）+ `obs.NewTeeHandler`（控制台 + OTLP，脱敏在外层）；默认关闭，单测覆盖（`otelx/logs_test.go`、`obs/otel_metrics_test.go`、`obs/tee_test.go`）
- ◐ 监控栈（Prometheus/Grafana 或云监控）后端接入与面板/规则生效（Collector 接收与导出后端配置随 S1 环境）

## 6. 灰度发布

- ✅ 特性开关：按用户/白名单/比例（稳定哈希不抖动）；后台 `/feature-flags` + admin-web 页；关闭即回滚
- ◐ 小程序体验版 → 审核 → 全量；灰度批次与观测门槛执行（外部/运营）

## 7. 验收与追溯

- ✅ PRD 11.4 逐项核对（`docs/prd-trace/README.md`，16 组；2026-10-17 复核：运营商/渠道/RBAC ❌→✅）
- ✅ 待确认事项复核（`docs/prd-trace/pending-confirmations.md`，22 项状态）
- ✅ 上线检查单（`docs/ops/launch-checklist.md`）
- ◐ 签署（安全/压测/备份/告警/回滚/业务验收）待外部；#13/#3/#4/#2/#6 已确认，生产 LLM 经独立网关接入（ADR-003）

## 剩余阻塞（按性质）

| 类型 | 项 | 解除条件 |
| --- | --- | --- |
| ~~产品/管理决策~~ | ~~p0 #13 五角色~~ **已确认**（RBAC 可配，`ADR-002`）→ p4 开工前置解除 | ✅ 已闭环（2026-10-17） |
| ~~p4 实施~~ | 运营商/渠道/RBAC/审计/认证/后台 | ✅ 已闭环（2026-10-17，`p4-channel-admin` 59/59） |
| ~~产品决策~~ | ~~p0 #3 AI 是否纳入一期~~ **已确认**：纳入一期文本场景（含客服），语音二期 | ✅ 已闭环（2026-10-17）；生产 LLM 待 #4 |
| 外部环境 | S1 正式压测、监控栈接入、云 PITR 演练、渗透测试 | 独立环境与工具 |
| 外部平台 | 小程序审核与灰度发版（含圈分享落地页） | 微信平台流程 |
| 产品口径 | 批量处置范围、圈名片图片口径 | 产品确认（不影响单条处置） |
| 合规/技术 | OTel SDK 引入 | 网络可用（当前离线/不稳） |

## 结论

- p6 在**代码可验证范围内已全部闭环**（安全测试、降级矩阵、可观测口径与告警、本地压测基线、备份与停机演练）。
- **p4 已全部完成（59/59）**：PRD 11.4 中运营商/渠道/RBAC/审计/认证/后台条目均已落地并通过集成测试，原阻塞解除。
- **8.1 仍为 ◐**：仅剩 3 项非代码事项——批量处置范围（产品）、圈名片图片口径（产品）、圈分享落地页（小程序发版）；单条处置与分享路径已具备。
- 建议：产品确认上述口径并完成小程序发版；随后执行 S1 压测与监控接入，正式满足 8.1/8.2/8.3。
