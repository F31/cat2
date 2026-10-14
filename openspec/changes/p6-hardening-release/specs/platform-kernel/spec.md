# platform-kernel 增量规格（p6-hardening-release，运维面）

## ADDED Requirements

### Requirement: 健康检查与优雅停机
系统 SHALL 提供 `/healthz`（存活）与 `/readyz`（依赖就绪）；停机 SHALL 先摘流量再排空任务（事件与调度任务排尽后退出）。

#### Scenario: 优雅停机
- **WHEN** 收到停机信号
- **THEN** readyz 先转为不就绪摘流量，队列排空后进程退出

### Requirement: 降级矩阵
系统 SHALL 实现并可演练以下降级：AI 不可用→原表单；搜索适配器故障→PG 简单检索；内容安全超时→进审核中队列重试不放行；Redis 不可用→进程内缓存/限流；notice 故障→事件积压后重放不丢失。

#### Scenario: 内容安全超时
- **WHEN** Moderation 端口连续超时
- **THEN** 发布进入审核中队列重试，用户端不展示未检内容

### Requirement: 备份与恢复
系统 SHALL 具备 WAL 归档 + 每日全备；恢复演练 SHALL 达到 RPO≤5 分钟、RTO≤1 小时并留存演练记录。

#### Scenario: 恢复演练
- **WHEN** 按演练脚本从备份恢复到指定时间点
- **THEN** 数据丢失 ≤5 分钟且 1 小时内服务可用

### Requirement: 告警
系统 SHALL 上线以下告警（初始阈值可配）：P99>3s、错误率>1%、支付回调失败>0、履约重试超阈、审核 SLA 将超时、Outbox 积压与最老消息年龄超阈、审计断链。

#### Scenario: 支付回调失败告警
- **WHEN** 出现一笔支付回调验签失败
- **THEN** 告警立即触发（阈值>0）

### Requirement: 灰度放量
特性开关 SHALL 支持按用户/比例/白名单放量；每个灰度批次 SHALL 有明确回滚动作（关闭开关即回退）。

#### Scenario: 比例放量
- **WHEN** 某特性按 10% 用户放量
- **THEN** 命中比例稳定且关闭开关后立即全量回退
