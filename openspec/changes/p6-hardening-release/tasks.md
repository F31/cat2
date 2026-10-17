# p6-hardening-release · 任务清单

## 1. 安全测试

- [x] 1.1 越权矩阵自动化：`TestIntegrationPermissionMatrixFull`（游客/未实名/非会员/非管理员 × 17 条关键路径，断言 401/403 与错误码）；**修复后台越权漏洞**：`Route.Admin` 此前未强制，现由装配根 `requireAdmin` 统一硬门槛（非管理员 403），`/admin/v1/menus` 同步收紧；`idn_user.admin_role` + `IsAdmin/GrantAdmin` + `catcube grant-admin <手机号>` 运维引导（五角色细化仍归 p4）
- [ ] 1.2 渗透测试（外部或工具）：OWASP Top 10 覆盖
- [x] 1.3 支付回放：回调审计留痕（`trade:pay:notify:{invalid_sign,amount_mismatch,out_of_order,duplicate,ok}`，伪造/金额不符=Denied）；`ErrNotifyAmount`/`ErrNotifyOutOfOrder` 分类；集成测试 `TestIntegrationPayReplayAndForgery`（伪造/金额不符/正常/重复幂等/乱序）
- [x] 1.4 CI 安全门禁：`security` job（govulncheck 依赖漏洞扫描 + gitleaks 密钥扫描）；静态检查 job 改用 `make lint`（archcheck + logscan 日志敏感信息扫描 + vet）；test job 补 `pkg` 与 `tools/logscan` 测试
- [x] 1.5 抓取限频：`bff` 联系方式查看按用户/日计数（`CATCUBE_CONTACT_DAILY_LIMIT`，默认 200，TTL 至次日）超限 429/120008 并 `contact_scrape_suspected` 告警；指标 `catcube_contact_reveal_total`；集成测试 `TestIntegrationContactScrapeRateLimit`
- [x] 1.6 审计断链告警：`audit.VerifyForward/VerifyIncremental`（逐条重算 hash + 锚点连接校验）；`pg.Audit` 游标（`sys_settings:audit.verify.cursor`，首跑建基线）与 `ListChainAfter`；`kernel.audit_verify` 定时任务 → `catcube_audit_chain_ok` 指标 + `audit_chain_broken` 告警；修复 char(64) 补空格导致的 `LastHash` 取链偏差；单测（篡改/断链定位）+ 集成测试 `TestIntegrationAuditChainBrokenAlert`

## 2. 压测（tests/perf，独立库）

- [x] 2.1 造数脚本：`tests/perf/seed.sh`（1 万用户/名片、1 万内容 + 搜索投影、1 千圈（5 成员/圈）、100 立方（3 成员）；固定 ID 段幂等，须用独立压测库）；场景与指标口径见 `tests/perf/README.md`
- [x] 2.2 场景压测：内置 `tools/loadtest` + `make loadtest`；本地基线 + **S1 形态复跑**（api×2 + worker/scheduler + Redis + 独立压测库，并发 100）feed/search/detail 全 PASS，报告 `tests/perf/reports/2026-10-17-s1-local.md`
- [x] 2.3 指标验证：本地基线（feed P95 71ms / search P99 30ms / detail P99 777ms）与 S1 形态复跑（feed P95 60ms / search P99 19ms / detail P99 66ms，错误率 0%）均达标；报告 `2026-10-16-local.md`、`2026-10-17-s1-local.md`；云 S1 归档待独立环境
- [x] 2.4 事件积压与吞吐：`Bus.Relay` 批量幂等/批量确认（`MarkDoneBatch`/`SeenBatch`/`MarkBatch`，适配器可选实现，无则回退逐条）；`TestIntegrationOutboxBacklogDrain` 积压 5000 条实测 **18,483 条/秒**（优化前 283 条/秒）、零丢失零死信

## 3. S1 环境部署

- [x] 3.1 deploy/ S1 编排：`deploy/compose/docker-compose.s1.yml`（api×2 + worker×1 含 scheduler + PG 主从流复制(脚本 `pg-replica-entrypoint.sh`/`init-replication.sql`) + Redis + 备份边车）；`docker compose config` 校验通过；多副本 Job 单活由 PG 咨询锁保证
- [x] 3.2 健康检查与停机：`/healthz`（存活）/`/readyz`（就绪，DB 探活）接入 compose healthcheck（`catcube probe`）；优雅停机 `CATCUBE_DRAIN_SECONDS`（SIGTERM→readyz 503 摘流量→排空窗口→Shutdown 等在途请求；`ready` 改 atomic 消除数据竞争）；演练 `deploy/scripts/drain-drill.sh` 实测 PASS（129 请求 0×5xx、80×503、5s 退出）；流程见 `deploy/compose/operations.md`
- [x] 3.3 三层配置落地：启动配置（`kernel/config` 环境变量）/业务配置（`kernel/settings` + `pg.Settings` 存储 → `sys_settings`；后台 `GET/PUT /api/admin/v1/settings` + admin-web「业务配置」页，热生效，集成测试验证）/特性开关（`kernel/featureflag`→`sys_feature_flag` 灰度）；见 `operations.md`
- [x] 3.4 备份恢复演练：`deploy/scripts/pitr-drill.sh` 本机复跑 PASS（恢复行数符合目标点，RTO 104s；前次 111s）；排空演练 S1 形态 PASS（140 请求 0×5xx、503×83、5s 退出）；云环境演练待执行

## 4. 降级与可靠性演练（逐项自动化/半自动化）

- [x] 4.1 AI 降级：`ai` 模块能力全部受开关门禁（默认关闭）；关闭/上游故障→`030001`/503，原表单路径不受影响；`llm.DevClient` 以 `[FAULT]` 模拟上游超时；集成测试 `TestIntegrationAIFlagGateAndDegrade` 覆盖「关闭→原表单可用→白名单开启→草稿→采纳→故障降级」
- [x] 4.2 搜索降级：`adapters/pg.Search.Query` 先 pg_trgm，失败（扩展缺失/超时）自动回退 ILIKE 简单检索并告警；`NewSearchSimple` 强制降级态；集成测试 `TestSearchSimpleFallback`
- [x] 4.3 内容安全超时：`trust.Review` 返回 `OutcomePending`（机检不可用→建机检重试任务，内容保持「审核中」不放行）；content 提交按结论三态处理；`moderation.DevClient`（`[FAULT]` 模拟超时）+ 集成测试 `TestIntegrationModerationTimeoutGate`
- [x] 4.4 Redis 不可用回退：`adapters/redis`（标准库 RESP 客户端）+ `Fallback` 主备缓存（Redis 失败读写回退进程内 + 降级回调告警），`CATCUBE_REDIS_ADDR` 装配；单测（正常/降级往返）+ 集成测试（cube-home 聚合缓存）
- [x] 4.5 notice 降级：`notice.SetFault` 故障注入（演练/测试）→ 事件保持 Outbox 重试、业务动作照常成功；故障解除后重放补齐站内信且无新死信；集成测试 `TestIntegrationNoticeFailureDoesNotBlockBusiness`
- [x] 4.6 Outbox 监控：`eventbus.Bus.ReportStats` + `pg.Outbox.Stats`（积压/最老年龄/死信）→ 指标 `catcube_outbox_*` 与阈值告警（`CATCUBE_ALERT_OUTBOX_*`，`kernel.outbox_monitor` 任务）；`GET /metrics` 导出；死信后台重放（`/dead-letters/{id}/replay`）；集成测试 `TestIntegrationOutboxBacklogAlert`/`TestIntegrationMetricsEndpoint`
- [x] 4.7 履约兜底：`trd_fulfillment` 重试超阈/死信计数 → `catcube_fulfillment_stuck` 指标与 `fulfillment_retry_exceeded` 告警（阈值 `CATCUBE_ALERT_FULFILL_RETRY`）；后台 `/fulfillments/{orderID}/retry` 手动补偿（保持原排队次序）；集成测试 `TestIntegrationFulfillmentAlertAndManualCompensation`

## 5. 可观测完善

- [x] 5.1 OTel 全链路核查：trace/metric/log 统一入口，指标带 module 标签。`internal/adapters/otelx`：`Setup`（OTLP Trace/Metric）+ `SetupLogs`（OTLP Log）+ slog 桥接 `NewLogHandler`；`kernel/obs`：server span、`NewTeeHandler`（控制台 + OTel 双出口，脱敏在外层）、`RegisterOTelMetrics`（内置 `/metrics` 镜像为 OTel 异步 Gauge，逐条带 `module` 标签，按指标名前缀推导）；`cmd/catcube` 启用时装配三路。单测 `otelx/logs_test.go` + `obs/otel_metrics_test.go` + `obs/tee_test.go`。（实际 Collector 接收与后端面板仍随 S1 监控栈启用）
- [x] 5.2 日志结构化与脱敏：`obs` 统一 `WithRequestID/WithModule/WithUserID` + `RedactingHandler`（密钥字段屏蔽、手机号/证件号/姓名掩码、值内嵌手机号抹除）；入口 `slog.SetDefault(RedactingHandler)`；扫描脚本 `tools/logscan`（接入 `make lint`）；单测（脱敏）+ 扫描器测试；`make lint` 通过
- [x] 5.3 业务看板口径与埋点：实现漏斗指标（注册/匹配曝光与点击/联系发起/会员转化/猫立方开通/支付成功与回调失败/审核积压与 SLA/Outbox/审计链/履约兜底），经 `GET /metrics` 暴露；口径文档 `docs/ops/dashboards.md`；集成测试 `TestIntegrationBusinessMetrics`（Grafana 面板随 S1 监控栈接入）
- [x] 5.4 告警清单与初始阈值：应用层已实现（`outbox_backlog_high`/`outbox_oldest_age_high`/`outbox_dead_high`/`fulfillment_retry_exceeded`/`review_sla_overdue`/`review_backlog_high`/`audit_chain_broken`/`contact_scrape_suspected`，阈值经 `CATCUBE_ALERT_*` 可配）；P99/错误率/DB/Redis 等接入层告警与处置动作见 `docs/ops/alerts.md`

## 6. 灰度发布

- [x] 6.1 特性开关放量机制：`kernel/featureflag`（`sys_feature_flag`；Enabled/白名单用户/比例放量，按用户稳定哈希不抖动）；后台 `GET/PUT /api/admin/v1/feature-flags` + 客户端 `GET /api/mp/v1/flags`（按用户评估，开关即回滚动作）；admin-web「特性开关」页；单测 + 集成测试覆盖
- [ ] 6.2 小程序体验版 → 微信审核 → 全量的发布流程走通（订阅消息模板、内容安全均已就位）
- [x] 6.3 灰度计划：`docs/ops/gray-release-plan.md`（B0 白名单→5%→20%→50%→100%，统一门槛：错误率/延迟/支付成功率/漏斗/Outbox/审核 SLA/审计链/履约；暂停与回滚动作）

## 7. PRD 11.4 验收

- [x] 7.1 PRD 11.4 逐项核对：`docs/prd-trace/README.md`（从 PRD docx §11.4 抽取全部验收点，逐条标注 ✅/◐/❌ 与代码/测试证据）
- [x] 7.2 追溯矩阵：`docs/prd-trace/README.md`（账号入驻/首页搜索/详情联系/消息/圈子/猫立方/认证/发布/会员支付/运营商/后台/跨端/企业版/触达/邀请/画像 16 组，逐条状态与证据）
- [x] 7.3 待确认复核：`docs/prd-trace/pending-confirmations.md`（22 项逐条给出当前实现口径/证据；已闭环 #8/#9/#14/#15/#17/#20；阻塞项 #13；外部项 #2/#3/#4/#6/#7）
- [x] 7.4 上线检查单：`docs/ops/launch-checklist.md`（安全/压测/部署/可观测/灰度/范围六节逐项勾选与证据；签署栏待负责方签署）

## 8. 退出标准

> 状态汇总见 `docs/prd-trace/p6-exit-report.md`（代码可验证范围内已闭环；以下三项待外部条件）。

- [x] 8.1 达到 PRD 11.4 验收标准：p4 运营商/渠道/RBAC + 圈子批量封禁/归档、圈名片（Canvas+小程序码）、分享落地页「来自 XX 圈」全部落地并有集成测试；真机保存/扫码/分享入口随小程序发版验证
- [ ] 8.2 S1 环境就绪且降级/备份演练全部通过（◐：**已在 4C8G 云主机实际部署** PG+Redis+API+worker+备份，健康/管理 API/Nginx 验证通过；宿主形态 S1 压测/排空/PITR 全 PASS；生产适配器、域名/TLS 与监控后端待补）
- [ ] 8.3 灰度放量完成，核心指标（注册→画像→匹配→联系→付费漏斗）可观测（◐：漏斗指标与 `/metrics` 口径就绪；监控栈接入与灰度执行待外部）
