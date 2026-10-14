# admin-web 增量规格（p5-ai-scenarios）

## ADDED Requirements

### Requirement: 提示词与用量管理
后台 SHALL 提供提示词版本管理（变更须过评测门禁）与用量页（按场景/模型的调用量、采纳率、成本）；成本异常与采纳率骤降 SHALL 触发告警。

#### Scenario: 采纳率告警
- **WHEN** 某场景采纳率低于阈值
- **THEN** 产生告警并在看板可见
