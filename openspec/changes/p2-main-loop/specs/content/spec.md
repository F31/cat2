# content 增量规格（p2-main-loop）

## ADDED Requirements

### Requirement: ContentKind 插件与四类内容
系统 SHALL 以 `ContentKind` 插件承载项目/产品/会议/活动四类内容：共有字段在信封表 `cnt_item`，独有字段在各类型子表（可筛选排序用类型化列，仅展示的少量扩展用 JSONB）；四类表单字段各自独立，产品 SHALL NOT 沿用项目字段名；新增内容类型 SHALL 只新增一个 Kind 与一张子表，不改 discovery/trust/circle/cube。

#### Scenario: 四类字段独立
- **WHEN** 分别提交项目（8 字段）与产品（11 字段）表单
- **THEN** 各自按其 Kind 的 schema 校验入库，互不串字段

### Requirement: 内容状态机
内容状态 SHALL 按状态机 #2 流转：审核中 → 展示中 → 已下架；已下架 SHALL NOT 允许自行恢复，可申诉；状态变更 SHALL 由 trust 审核结论事件驱动，trust 不直接改内容表。

#### Scenario: 举报属实下架
- **WHEN** trust 发布 `ReportConfirmed`
- **THEN** content 将内容转「已下架」并记录原因

### Requirement: 发布门槛与会员可见字段
发布内容 SHALL 要求已实名；项目/产品的「核心优势」与「合作联系」字段 SHALL 仅会员可见，非会员请求返回中不包含且给出解锁 Reason。

#### Scenario: 非会员查看详情
- **WHEN** 非会员查看项目详情
- **THEN** 核心优势与合作联系不返回，前端 PolicyGate 显示开通会员引导
