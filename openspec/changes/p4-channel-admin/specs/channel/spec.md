# channel 增量规格（p4-channel-admin）

## ADDED Requirements

### Requirement: 运营商开户与状态机
运营商 SHALL 经后台开户审核开通（区域查重一地一家）；状态按 #16 流转：申请中→已开通→已停用，停用后须重新申请；代理费（首年 3 万/次年 2 万）与保证金（1 万可退）数值 SHALL 后台可配，保证金 SHALL 记预收款台账不与收入混记。

#### Scenario: 区域查重
- **WHEN** 在已有运营商的区域再开户
- **THEN** 审核被拦截并提示一地一家

### Requirement: 佣金计提与比例快照
系统 SHALL 订阅 `OrderPaid`/`MemberActivated` 按配置比例（初始首单 30%/续费 10%，后台可配）生成佣金流水；每笔流水 SHALL 写入 ratio_snapshot，配置变更不影响历史流水；计佣范围（首单/续费、是否含立方订阅）以配置为准。

#### Scenario: 比例变更隔离
- **WHEN** 佣金比例从 30% 调整为 25%
- **THEN** 既有流水的计提金额不变，新订单按新比例

### Requirement: 结算单复核与执行分离
佣金月结 SHALL 按 #17 流转：待结算→已结算→已打款，打款不可撤销；同一结算单的复核人与执行（打款）人 SHALL NOT 为同一人，由内核策略强制互斥，违反记 denied 审计；打款 SHALL 留凭证。

#### Scenario: 复核执行同人拒绝
- **WHEN** 结算单复核人尝试执行打款
- **THEN** 系统拒绝并记审计

### Requirement: 多级分销红线
运营商数据模型 SHALL 只有 `referrer_operator_id` 单列：无上级字段、不能自建下级；任何接口 SHALL NOT 暴露多级关系。

#### Scenario: schema 防线
- **WHEN** 审查 channel 表结构与 contract
- **THEN** 不存在上级/下级字段或多级计佣路径

### Requirement: 保底任务跟踪
系统 SHALL 跟踪运营商年度保底（12 个月 ≥50 付费会员或 ≥3 落地项目），进度可查询并对未达标预警。

#### Scenario: 保底进度
- **WHEN** 运营商或渠道经理查看业绩概览
- **THEN** 显示新增付费会员、佣金合计与保底完成度
