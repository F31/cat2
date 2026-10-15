# p6-hardening-release · 任务清单

## 1. 安全测试

- [ ] 1.1 越权测试：权限矩阵全部 denied 路径自动化（游客/注册/会员/各后台角色 × 关键资源）
- [ ] 1.2 渗透测试（外部或工具）：OWASP Top 10 覆盖
- [ ] 1.3 支付回放测试：重复回调、伪造签名、乱序回调全部被拒绝且记审计
- [ ] 1.4 依赖漏洞扫描与 SAST 接入 CI
- [ ] 1.5 联系方式抓取压测：模拟批量查看联系方式，验证按用户/日阈值限频与告警触发
- [ ] 1.6 审计断链告警验证：人为篡改一条 sys_audit，断链检测告警触发

## 2. 压测（tests/perf，独立库）

- [ ] 2.1 数据准备：万级用户、万级内容、千级圈子、百级猫立方的造数脚本
- [ ] 2.2 场景压测：feed 聚合、搜索、详情聚合、消息同步、下单支付（mock 渠道）
- [ ] 2.3 指标验证：峰值并发百级 + 5~10 倍余量；首屏 ≤3 秒；常规查询 ≤1 秒；出压测报告
- [ ] 2.4 事件积压压测：Outbox 高峰积压下投递延迟与恢复时间达标

## 3. S1 环境部署

- [x] 3.1 deploy/ S1 编排：`deploy/compose/docker-compose.s1.yml`（api×2 + worker×1 含 scheduler + PG 主从流复制(脚本 `pg-replica-entrypoint.sh`/`init-replication.sql`) + Redis + 备份边车）；`docker compose config` 校验通过；多副本 Job 单活由 PG 咨询锁保证
- [x] 3.2 `/healthz`（存活）、`/readyz`（就绪，DB 探活）已接入 api healthcheck；停机流程（摘流量→排空→停依赖）见 `deploy/compose/operations.md`
- [x] 3.3 三层配置落地：启动配置（`kernel/config` 环境变量）/业务配置（`kernel/settings`→`sys_settings` 热生效）/特性开关（`kernel/featureflag`→`sys_feature_flag` 灰度）；见 `operations.md`
- [ ] 3.4 备份策略生效：WAL 归档 + 每日全备；恢复演练达到 RPO≤5 分钟、RTO≤1 小时并留记录

## 4. 降级与可靠性演练（逐项自动化/半自动化）

- [ ] 4.1 AI 不可用 → 入驻/发布/搜索走原表单（关闭开关回归）
- [x] 4.2 搜索降级：`adapters/pg.Search.Query` 先 pg_trgm，失败（扩展缺失/超时）自动回退 ILIKE 简单检索并告警；`NewSearchSimple` 强制降级态；集成测试 `TestSearchSimpleFallback`
- [x] 4.3 内容安全超时：`trust.Review` 返回 `OutcomePending`（机检不可用→建机检重试任务，内容保持「审核中」不放行）；content 提交按结论三态处理；`moderation.DevClient`（`[FAULT]` 模拟超时）+ 集成测试 `TestIntegrationModerationTimeoutGate`
- [x] 4.4 Redis 不可用回退：`adapters/redis`（标准库 RESP 客户端）+ `Fallback` 主备缓存（Redis 失败读写回退进程内 + 降级回调告警），`CATCUBE_REDIS_ADDR` 装配；单测（正常/降级往返）+ 集成测试（cube-home 聚合缓存）
- [ ] 4.5 notice 故障 → 业务不受影响，事件积压后重放无丢失
- [x] 4.6 Outbox 监控：`eventbus.Bus.ReportStats` + `pg.Outbox.Stats`（积压/最老年龄/死信）→ 指标 `catcube_outbox_*` 与阈值告警（`CATCUBE_ALERT_OUTBOX_*`，`kernel.outbox_monitor` 任务）；`GET /metrics` 导出；死信后台重放（`/dead-letters/{id}/replay`）；集成测试 `TestIntegrationOutboxBacklogAlert`/`TestIntegrationMetricsEndpoint`
- [x] 4.7 履约兜底：`trd_fulfillment` 重试超阈/死信计数 → `catcube_fulfillment_stuck` 指标与 `fulfillment_retry_exceeded` 告警（阈值 `CATCUBE_ALERT_FULFILL_RETRY`）；后台 `/fulfillments/{orderID}/retry` 手动补偿（保持原排队次序）；集成测试 `TestIntegrationFulfillmentAlertAndManualCompensation`

## 5. 可观测完善

- [ ] 5.1 OTel 全链路核查：trace/metric/log 统一入口，指标带 module 标签
- [x] 5.2 日志结构化与脱敏：`obs` 统一 `WithRequestID/WithModule/WithUserID` + `RedactingHandler`（密钥字段屏蔽、手机号/证件号/姓名掩码、值内嵌手机号抹除）；入口 `slog.SetDefault(RedactingHandler)`；扫描脚本 `tools/logscan`（接入 `make lint`）；单测（脱敏）+ 扫描器测试；`make lint` 通过
- [ ] 5.3 业务看板上线：注册、画像完成度、匹配点击、联系发起、会员转化、支付成功率、猫立方开通、审核积压
- [ ] 5.4 告警上线（初始值可配）：P99>3s、错误率>1%、支付回调失败>0、履约重试超阈、审核 SLA 将超时

## 6. 灰度发布

- [x] 6.1 特性开关放量机制：`kernel/featureflag`（`sys_feature_flag`；Enabled/白名单用户/比例放量，按用户稳定哈希不抖动）；后台 `GET/PUT /api/admin/v1/feature-flags` + 客户端 `GET /api/mp/v1/flags`（按用户评估，开关即回滚动作）；admin-web「特性开关」页；单测 + 集成测试覆盖
- [ ] 6.2 小程序体验版 → 微信审核 → 全量的发布流程走通（订阅消息模板、内容安全均已就位）
- [ ] 6.3 灰度计划：内测（白名单）→ 小比例 → 全量，每批次观测指标门槛

## 7. PRD 11.4 验收

- [ ] 7.1 逐项核对 PRD 11.4 验收标准（含注册登录一体化、未填手机号不能进入主流程）
- [ ] 7.2 PRD 追溯矩阵（docs/prd-trace/）逐项标注落地状态与测试证据
- [ ] 7.3 待确认事项遗留清单复核：p0 闭环结论与实现一致
- [ ] 7.4 上线检查单签署（安全/压测/备份/告警/回滚预案）

## 8. 退出标准

- [ ] 8.1 达到 PRD 11.4 验收标准
- [ ] 8.2 S1 环境就绪且降级/备份演练全部通过
- [ ] 8.3 灰度放量完成，核心指标（注册→画像→匹配→联系→付费漏斗）可观测
