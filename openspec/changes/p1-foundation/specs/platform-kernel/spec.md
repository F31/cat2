# platform-kernel 增量规格（p1-foundation）

## ADDED Requirements

### Requirement: 模块装载与依赖拓扑
系统 SHALL 在启动时按 `DependsOn` 与依赖阶梯（R0→R6，同阶不可互依）对模块做拓扑排序装载；缺失依赖时启动必须失败，不得在运行时才报错。

#### Scenario: 缺失依赖启动失败
- **WHEN** 装载 cube 而未装载其依赖的 content
- **THEN** 进程启动失败并输出缺失依赖清单

#### Scenario: 空模块按阶梯装载
- **WHEN** 一个仅有 contract 与 module.go 的空模块声明合法依赖
- **THEN** 它能被装载并通过自身测试

### Requirement: Outbox 事件投递
系统 SHALL 将业务表变更与 `sys_outbox` 事件在同一事务写入，以至少一次语义投递；订阅方 SHALL 以事件 ID 幂等去重；失败事件 SHALL 可重试，超过阈值进死信并可在后台重放。

#### Scenario: 同事务一致性
- **WHEN** 业务事务提交
- **THEN** 对应事件已在 `sys_outbox` 中，不存在「业务已改但事件丢失」

#### Scenario: 订阅方幂等
- **WHEN** 同一事件 ID 被投递两次
- **THEN** 订阅方只处理一次

### Requirement: 状态机引擎
系统 SHALL 以声明式 `StateMachineDef{from,event,to,actor,guard}` 执行状态流转；非法转移 SHALL 在引擎层被拒绝并返回 Reason；每次流转 SHALL 写状态日志并发布事件。自动动作只允许「到期转态」，不允许「自动通过」。

#### Scenario: 非法转移拒绝
- **WHEN** 对「已下架」内容发起「发布」转移
- **THEN** 引擎拒绝，状态不变，返回非法转移 Reason

### Requirement: 配额引擎
系统 SHALL 提供 `QuotaDef{key,window,scope}` 注册机制；限额数值 SHALL 来自 settings 配置而非硬编码；计数 SHALL 落 `sys_quota_counter` 且自然日窗口按 Asia/Shanghai 计算；超限 SHALL 返回带 Reason 的拒绝。

#### Scenario: 限额配置化
- **WHEN** 后台把某 QuotaDef 数值从 3 改为 5
- **THEN** 不发版即生效，并留下配置版本与操作人审计

### Requirement: 配置三层与 SettingsSchema
系统 SHALL 支持启动配置 / 业务配置（`sys_settings`，带类型校验、版本、生效时间、操作人审计）/ 特性开关三层；业务配置项 SHALL 由模块以 `SettingsSchema` 声明，后台据此自动生成配置界面；scope 支持 platform/space，取值顺序 space → platform（一期 space 恒为 1）。

#### Scenario: 新增配置项不写前端
- **WHEN** 某模块在 SettingsSchema 登记一个新键
- **THEN** 后台配置中心自动出现该项，无需新增前端代码

### Requirement: ScopedRepo 数据范围
所有仓储访问 SHALL 经 `ScopedRepo` 并强制注入 `Scope`（platform/space/org/cube/circle/self）；调用方 SHALL 无法获得绕过 Scope 的裸查询；已关闭/已删除实体 SHALL 默认被过滤，后台查询须显式声明。

#### Scenario: 默认过滤已关闭实体
- **WHEN** 查询内容列表且目标 `status=已下架`
- **THEN** 默认结果集不包含该记录

### Requirement: 敏感字段保护
系统 SHALL 对手机号/证件号/联系方式使用 AES-256-GCM 字段加密与 phone_hmac 盲索引（独立密钥）；对外 DTO 中敏感字段 SHALL 以 `Visible[T]` 包装，漏写策略的默认结果是「不输出」而非泄漏。

#### Scenario: 盲索引唯一校验
- **WHEN** 用已注册手机号再次注册
- **THEN** 系统经 phone_hmac 命中唯一冲突并提示已注册，全程不明文比较

### Requirement: 审计哈希链
系统 SHALL 追加写 `sys_audit`（含 actor_type/channel/action/target_ref/detail 快照/result/ip/user_agent/request_id），每条记录 SHALL 含 prev_hash 与 hash=SHA256(prev_hash+canonical) 构成防篡改链；发现断链 SHALL 告警。越权拒绝（denied）也 SHALL 记录。

#### Scenario: 越权尝试留痕
- **WHEN** 无权限用户访问审核队列被拒
- **THEN** sys_audit 新增一条 result=权限拒绝 的记录，含触发规则
