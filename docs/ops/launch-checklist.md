# 上线检查单（p6 §7.4）

> 状态口径：`[x]` 已完成并可复核（含证据）；`[ ]` 待办/待外部环境；`外部` 需负责方签署。
> 本清单随 p4/p5 状态更新；签署在 p6 退出时进行。

## 1. 安全

- [x] 越权矩阵自动化（游客/未实名/非会员/非管理员 × 关键资源）→ `TestIntegrationPermissionMatrixFull`
- [x] 后台路由管理员硬门槛（`requireAdmin`；`Route.Admin` 强制）→ 越权漏洞已修复
- [x] 支付回放/伪造/乱序回调拒绝且记审计 → `TestIntegrationPayReplayAndForgery`
- [x] 日志敏感信息扫描（session_key/手机号/证件号不入日志）→ `tools/logscan`（`make lint`）
- [x] 审计哈希链断链/篡改告警 → `kernel.audit_verify` + `TestIntegrationAuditChainBrokenAlert`
- [x] 依赖漏洞扫描与密钥扫描接入 CI（govulncheck + gitleaks）
- [ ] 渗透测试（外部或工具，OWASP Top 10）→ `外部`
- [ ] 联系方式抓取压测（批量场景）→ 需独立环境（限频与告警已实现）

## 2. 压测与容量

- [x] 造数脚本（万级用户/内容、千级圈、百级立方）→ `tests/perf/seed.sh`
- [ ] 场景压测（feed/搜索/详情/消息/下单支付）与指标达标（首屏 P95 ≤3s、常规查询 P99 ≤1s、百级并发 + 5~10 倍余量）→ 需 S1 独立环境
- [ ] 事件积压压测（Outbox 投递延迟与恢复）→ 需独立环境
- [ ] 压测报告归档 `tests/perf/reports/`

## 3. 部署与可靠性

- [x] S0 一键起（`make up`）；S1 编排（api×2 + worker/scheduler + PG 主从 + Redis）
- [x] `/healthz`、`/readyz` 接入负载探针；停机流程（先摘流量再排空）文档化
- [x] PITR 备份配置（WAL 归档 + 每日全备 + 保留清理）
- [x] 备份恢复演练脚本与实测通过 → `deploy/scripts/pitr-drill.sh`（本机实测 PASS，分批写入 → 时间点恢复精确到基线）
- [ ] 云环境 PITR 演练（RPO ≤5 分钟、RTO ≤1 小时）并留记录 → `外部`（云托管 DB 执行）
- [x] 降级矩阵：搜索回退、内容安全超时、notice 故障、Redis 回退（自动化演练）
- [x] 多副本 scheduler 单活（PG 咨询锁）

## 4. 可观测

- [x] 结构化日志（request_id/module/user_id，脱敏）
- [x] 业务看板口径与指标（`/metrics`）→ `docs/ops/dashboards.md`
- [x] 告警清单与初始阈值 → `docs/ops/alerts.md`
- [ ] 监控栈接入（Prometheus/Grafana 或云监控）与面板/告警规则生效 → `外部`
- [x] OTel 接入（p1 §5.6 / p6 §5.1）→ trace/metric/log 统一入口：`internal/adapters/otelx`（`Setup`/`SetupLogs`）+ `kernel/obs`（server span、`NewTeeHandler`、`RegisterOTelMetrics`，指标带 `module` 标签）；S1 `otel-collector` 服务与 `deploy/compose/otel-collector.yaml`；默认关闭（`CATCUBE_OTEL_ENABLED`），启用步骤见 `docs/ops/dashboards.md`
- [ ] 监控栈（Prometheus/Grafana 或云监控）抓取 `/metrics` 与 Collector `:8889` 并配置面板/告警 → 待环境

## 5. 灰度与回滚

- [x] 特性开关（按用户/比例/白名单；关闭即回滚）
- [ ] 小程序体验版 → 审核 → 全量流程 → 外部（微信平台）
- [ ] 灰度批次与观测指标门槛执行 → 运营

## 6. 范围与验收

- [x] PRD 11.4 逐项核对与证据 → `docs/prd-trace/README.md`
- [x] 待确认事项复核（22 项状态）→ `docs/prd-trace/pending-confirmations.md`
- [x] 待确认 #13（后台五角色）闭环 + p4 全部落地（59/59：运营商/渠道/佣金/结算/RBAC/审计/认证/后台页面）
- [x] p5（AI 一期）已确认纳入文本场景（ADR-003 网关；开关默认关，放量为配置动作）
- [x] AI 降级与开关门禁验证（关闭/上游故障 → `030001`，原表单不受影响）
- [x] 审计体系（脱敏/装饰器/敏感路由声明校验/查询页/denied 告警/留存归档）
- [x] 认证全类型与组织管理员机制、标识配置、导出中心（水印+审计）、看板与埋点
- [ ] 上线检查单签署（安全/压测/备份/告警/回滚）→ `外部`

## 签署

| 项 | 负责方 | 结论 | 日期 |
| --- | --- | --- | --- |
| 安全 | 研发 + 外部渗透 | | |
| 压测 | 研发 | | |
| 备份恢复 | 研发 + 运维 | | |
| 告警/可观测 | 研发 + 运维 | | |
| 回滚预案 | 研发 + 产品 | | |
| 业务验收 | 产品 + 业务方 | | |
