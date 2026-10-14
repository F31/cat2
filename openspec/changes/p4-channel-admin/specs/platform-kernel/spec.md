# platform-kernel 增量规格（p4-channel-admin）

## ADDED Requirements

### Requirement: 复核与执行互斥
内核 policy SHALL 支持「复核≠执行」互斥规则：同一结算单/退款单的复核人与执行人不得为同一人；违反 SHALL 拒绝并记 denied 审计。

#### Scenario: 同人互斥
- **WHEN** 同一后台账号对同一单据先复核后执行
- **THEN** 执行被拒并留痕

### Requirement: 审计点注册表与 CI 校验
模块 SHALL 以 `AuditPoint{action,级别,记失败否}` 声明审计点；CI SHALL 校验：声明了 action 的 handler 必须接审计装饰器；出现敏感动词（refund/ban/export/config/transfer/proxy）而未声明审计点的构建 SHALL 失败。

#### Scenario: 未声明即失败
- **WHEN** 新增一个 refund 相关 handler 但未声明 AuditPoint
- **THEN** CI 失败并指出缺失的审计点

### Requirement: 埋点最小化
一期行为埋点 SHALL 仅采集漏斗必需 8 事件（register/profile_complete/feed_click/search/detail_view/contact_initiate/member_quote/order_paid），事件名与属性在 contracts/telemetry 登记；埋点数据 SHALL 以独立 `tlm_` 前缀与业务数据分离存储。

#### Scenario: 未登记埋点拒绝
- **WHEN** 提交一个未在 contracts/telemetry 登记的埋点事件
- **THEN** 契约校验失败
