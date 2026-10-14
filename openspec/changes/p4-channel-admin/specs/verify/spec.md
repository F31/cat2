# verify 增量规格（p4-channel-admin，认证全类型）

## ADDED Requirements

### Requirement: 五类认证与审核路由
系统 SHALL 支持五类认证（实名/个人/个人企业身份/企业/机构，`cert_type` 区分）；平台审核类 SHALL 在 1-3 工作日内处理；他方审核类（个人企业身份认证由企业审、身份核验由组织管理员审）SHALL 先到先处理，超 7 天未处理平台可代处理或提醒；驳回 SHALL 必填原因且可修改重提。

#### Scenario: 超期代处理
- **WHEN** 核验申请超 7 天未处理
- **THEN** 平台管理员可代处理，审计记 verify:proxy-review 并告警

### Requirement: 组织管理员机制
首任组织管理员 SHALL 为组织入驻申请人；添加管理员 SHALL 经对方确认；管理员失联可申请平台重置（二次确认+审计）；组织认证未通过的组织 SHALL NOT 能审核成员核验。

#### Scenario: 未认证组织审核拦截
- **WHEN** 未通过组织认证的组织管理员尝试审核核验
- **THEN** 系统拒绝并记 denied 审计

### Requirement: 一人多单位与 Badge
系统 SHALL 以 `vfy_affiliation` 支持一人多单位（is_primary 主单位展示在名字后，其他在下面）；未核验的单位身份 SHALL 置灰展示，核验通过 SHALL 显示红框标识；认证标识 SHALL 经统一 Badge 机制输出 {code,label,level}（含「猫平方官方合作认证」），新增标识类型只加配置与样式不改接口。

#### Scenario: 多单位展示
- **WHEN** 用户有两个已核验单位
- **THEN** 卡片名字后显示主单位，其余列于下方
