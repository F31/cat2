# ai 增量规格（p5-ai-scenarios）

## ADDED Requirements

### Requirement: 可选装载与降级
`ai` 模块 SHALL 可选装载；未装载或不可用时应注入 Noop 实现，调用方收到「不可用」即走原手动路径；关闭开关 SHALL NOT 影响任何既有链路。

#### Scenario: 关闭开关回归
- **WHEN** modules.enabled 不含 ai 启动全链路测试
- **THEN** 入驻、发布、搜索、客服全部走手动路径且测试通过

### Requirement: 输出收敛
AI 输出 SHALL 统一收敛到 `TagSet`（经 `dict.ValidateTagSet`，模型只能输出合法 code，非法丢弃）与各 ContentKind 字段 schema（结构化输出 + JSON Schema 校验，不合规重试一次后降级）。

#### Scenario: 非法 code 丢弃
- **WHEN** 模型输出了不存在的行业 code
- **THEN** 该字段被丢弃，其余合法字段保留，用户可手动补全

### Requirement: 隐私最小化与审计
手机号、证件号、联系方式 SHALL NOT 进入提示词与向量；每次 AI 调用 SHALL 记审计（场景、模型、token、耗时、采纳与否）。

#### Scenario: 敏感字段过滤
- **WHEN** 构造提示词的输入含联系方式
- **THEN** 统一入口过滤该字段，提示词中不出现

### Requirement: 人在回路
AI 产出 SHALL 仅为草稿/建议：入驻与发布由用户确认后才写入；审核辅助只做预分类与摘要，处罚 SHALL 必须人工确认。

#### Scenario: 草稿确认
- **WHEN** AI 生成发布草稿
- **THEN** 用户编辑确认后才进入提交流程，AI 输出不直接落库

### Requirement: 评测门禁
系统 SHALL 维护固定样本集评测（抽取准确率、TagSet 合法率、降级路径）；提示词变更 SHALL 通过评测才能生效；采纳率与成本 SHALL 可观测并设告警。

#### Scenario: 提示词变更
- **WHEN** 后台提交新提示词版本但评测未达标
- **THEN** 版本不生效并提示评测结果
