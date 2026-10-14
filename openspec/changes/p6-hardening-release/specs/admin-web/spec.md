# admin-web 增量规格（p6-hardening-release）

## ADDED Requirements

### Requirement: 死信重放与任务监控
后台 SHALL 提供 Outbox 积压与死信查看、重放入口（`sys:job-replay` 权限点，记审计）；任务执行状态（sys_job）与最老消息年龄 SHALL 可视化。

#### Scenario: 死信重放
- **WHEN** 超管在后台重放一条死信事件
- **THEN** 事件重新投递成功，审计记录操作人与事件 ID
