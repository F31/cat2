# admin-web 增量规格（p2-main-loop，基础管理）

## ADDED Requirements

### Requirement: AdminSection 注册组装
后台菜单与路由 SHALL 由各模块 `AdminSection` 声明、admin 应用组装；本阶段交付基础管理：用户列表与封禁、内容管理、审核队列、订单、会员列表。

#### Scenario: 菜单按声明组装
- **WHEN** 某模块注册 AdminSection
- **THEN** 后台出现对应菜单与路由，无需改后台壳代码

### Requirement: 审核队列操作
审核员 SHALL 能在统一审核队列按 SLA 排序处理任务（通过/驳回须填原因）；处罚类结论 SHALL 人工确认后生效。

#### Scenario: 驳回必填原因
- **WHEN** 审核员驳回但未填原因
- **THEN** 提交被拒绝并提示填写驳回原因
