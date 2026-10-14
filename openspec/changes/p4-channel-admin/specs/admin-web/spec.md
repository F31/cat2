# admin-web 增量规格（p4-channel-admin）

## ADDED Requirements

### Requirement: 后台 RBAC 五角色与操作级权限点
系统 SHALL 内置五个后台角色（超级管理员/运营/审核员/财务/渠道经理）；权限点 SHALL 分菜单级与操作级两级，命名 `<module>:<resource>:<action>` 并统一登记；服务端强制校验 SHALL 是唯一权威（前端隐藏仅作体验）；后台账号 SHALL 强制二次验证；角色带 space_id。

#### Scenario: 越权操作拒绝
- **WHEN** 审核员调用资金类接口
- **THEN** 服务端拒绝并记 denied 审计，无论前端按钮是否可见

### Requirement: 敏感操作二次确认
退款、封禁、改价格与比例、导出、结算等敏感操作 SHALL 二次确认并写审计（detail 含操作前后值快照）。

#### Scenario: 退款二次确认
- **WHEN** 财务发起退款
- **THEN** 须二次确认，审计记录前后值快照与操作人

### Requirement: 用户 360 与代操作
治理分区 SHALL 提供用户 360 详情（聚合各模块只读接口，Tab 分区带模块来源标注）与 10 项运营代操作（代处理超期核验/重置组织管理员/主账号转移/强制解除子账号/履约补偿/空间封禁解散/归属纠错/会员人工调整双人复核/定向公告/死信重放）；代操作 SHALL 二次确认并全量审计。

#### Scenario: 代操作留痕
- **WHEN** 平台代处理一笔超期核验
- **THEN** 审计记录权限点 verify:proxy-review、前后值快照，并触发告警

### Requirement: 配置中心自动渲染
后台配置中心 SHALL 由各模块 `SettingsSchema` 自动渲染，新增可配置项 SHALL NOT 需要新增前端代码；配置变更 SHALL 带版本、生效时间与操作人审计。

#### Scenario: 改价留痕
- **WHEN** 超管修改会员年费锚点
- **THEN** 生成新配置版本并记审计，旧版本可追溯

### Requirement: 审计查询与告警
后台审计页 SHALL 支持 who/where/when/what/result 五维筛选、target_ref 反查、actor 正查与快照对比；系统 SHALL 对 denied 突增、登录锁定激增、联系方式查看超阈、审计断链告警（阈值可配）。

#### Scenario: 对象反查
- **WHEN** 以某结算单为 target_ref 查询
- **THEN** 列出该对象经历的全部操作（生成/复核/打款）及操作人
