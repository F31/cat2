# miniapp 增量规格（p5-ai-scenarios）

## ADDED Requirements

### Requirement: ai-entry 嵌入组件
小程序 SHALL 提供 `ai-entry` 组件嵌入入驻页、发布页、搜索页；`ai` 不可用或未装载时组件 SHALL 自动隐藏，不影响手动填写。

#### Scenario: 组件自动隐藏
- **WHEN** ai 模块未装载
- **THEN** 三个页面不渲染 ai-entry，手动路径完整可用
