# notice 增量规格（p3-collaboration）

## ADDED Requirements

### Requirement: 数据驱动通知
系统 SHALL 以各模块注册的 `NoticeTemplate`（事件→接收人规则→文案→渠道→是否合并）执行通知；PRD 11.3 的 36 条触达 SHALL 逐条映射为模板；业务模块 SHALL NOT 直接发通知；任何事件 SHALL 至少有一条站内信兜底；资金与审核类通知不可关闭。

#### Scenario: 新事件接入
- **WHEN** 某模块注册一条新 NoticeTemplate
- **THEN** 事件发生后通知自动触达，无需改 notice 代码

### Requirement: 订阅消息授权额度
系统 SHALL 以 `ntc_subscribe_quota` 管理订阅消息额度：在上下文动作处（关注/加入猫立方/提交任务）申请授权，发送时消耗；无额度 SHALL 降级为站内未读，并在下次进入小程序时提示开启。

#### Scenario: 额度耗尽降级
- **WHEN** 某模板额度为 0 时触达事件
- **THEN** 仅落站内通知，不产生订阅消息发送失败

### Requirement: 圈内通知合并
同一圈子同一自然日的多条发布通知 SHALL 合并为一条并累计条数（`ntc_digest` upsert + 日结任务）；发布者本人 SHALL NOT 收到自己的触达。

#### Scenario: 同圈同日合并
- **WHEN** 同圈同日有 4 条新发布
- **THEN** 圈成员收到 1 条合并通知显示 4 条
