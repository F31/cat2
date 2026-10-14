# dict 增量规格（p1-foundation）

## ADDED Requirements

### Requirement: 字典选项只停用不删除
系统 SHALL 以 `dimension + code` 为键管理 taxonomy 选项（行业/子行业、区域、行业性质、合作需求/供给、会议/活动领域等）；选项 SHALL 只停用不删除，历史数据保留展示；选项增减 SHALL 由运营自助完成，无需发版。

#### Scenario: 停用选项
- **WHEN** 运营停用某个行业选项
- **THEN** 新表单不再可选，历史实体仍正常展示该 label

#### Scenario: 行业两级联动
- **WHEN** 选择某行业
- **THEN** 子行业选项经 `parent_code` 联动过滤，仅两级

### Requirement: 主体类型可配置
系统 SHALL 以配置表管理主体类型（编码创建后不可改、名称、图标、是否可发布内容、是否首页可见、排序、状态）；新增第七类主体 SHALL 只加配置不发版。

#### Scenario: 新增主体类型
- **WHEN** 后台新增一条主体类型并选择字段模板
- **THEN** 入驻表单按模板渲染，无需发版

### Requirement: TagSet 统一校验
所有可发布/可检索实体 SHALL 嵌入同一 `TagSet{industry[],region,nature[],need[],offer[],domain[]}`（存 code）；系统 SHALL 经 `ValidateTagSet` 统一校验，非法或已停用 code SHALL 被拒绝。

#### Scenario: 非法 code 拒绝
- **WHEN** 提交的 TagSet 含不存在的 code
- **THEN** 校验失败并指出具体维度与 code
