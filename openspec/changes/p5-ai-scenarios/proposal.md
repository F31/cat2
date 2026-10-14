# p5-ai-scenarios · AI 一期场景

## Why

AI 能力在架构上早已预留落位（《技术方案 v2.5》第 9 章：`ai` 模块 R0、可选装载可降级、输出收敛到 TagSet 与 ContentKind 字段 schema），但一期场景属于「替代填表的辅助入口」，是需求范围变化，必须经产品确认（待确认 #3）与法务合规评估（待确认 #4：生成式 AI 备案、境内模型选择、数据出境）通过后方可开工；不纳入则本变更整体取消，`ai` 不装载，其余设计不受影响。前置阶段已交付主链路（入驻表单、四类内容发布、关键词搜索、审核队列），本阶段在其上以「增强入口」方式叠加四个一期场景：一句话入驻、发布智能草稿、自然语言搜索、智能客服，外加审核辅助（只做预分类与摘要，处罚必须人工确认，技术方案 8.3/详细设计 3.5）。退出标准（技术方案第 13 章 / 详细设计 9.2）：关闭开关全链路无影响；采纳率与成本可观测。

## What Changes

- 交付 `ai` 模块（R0，所有业务模块可依赖、不依赖任何业务模块）：LLM/Embedding/OCR/ASR 网关（`ports/llm`/`ports/ocr`/`ports/asr` + 供应商适配器）、提示词版本管理（`ai_prompt`）、配额与成本控制（QuotaDef + 预算 SettingsSchema）、调用全量审计（`ai_call_log`）
- 未装载时 `wire.go` 注入 Noop 实现，调用方拿到「不可用」走原手动路径，全链路不受影响；手机号/证件号/联系方式不进提示词与向量
- 四个一期场景：一句话入驻（语音/文字 → 画像字段与 TagSet）、发布智能草稿（一句话 → 四类 ContentKind 字段 schema 结构化输出 + JSON Schema 校验，不合规则重试或降级）、自然语言搜索（Translate2Query 转结构化查询）、智能客服（经产品确认）；审核辅助只做预分类与摘要
- 输出收敛：统一收敛到 TagSet（经 `dict.ValidateTagSet`，模型只能输出合法 code，否则丢弃）与各 ContentKind 字段 schema
- 小程序 `ai-entry` 组件嵌入入驻页/发布页/搜索页，`ai` 不可用或未装载时自动隐藏，不影响手动填写
- AI 评测门禁：固定样本集（抽取准确率、TagSet 合法率、降级路径），提示词变更必须过评测（详细设计 9.1）
- 后台「提示词与用量」管理页（详细设计 3.6 后台菜单）；采纳率与成本进看板与告警（技术方案 12.4）

## Impact

- **capabilities**：`ai`（新增模块）、`miniapp`（ai-entry 组件与三个页面嵌入、智能客服入口）、`admin-web`（提示词与用量管理页、AI 看板区块）
- **后端代码**：`apps/server/internal/modules/ai/`（contract/module.go/admin/migrations/internal 四层）、`internal/ports/{llm,ocr,asr}`、`internal/adapters/llm_<vendor>/`、`app/wire.go`（Noop 装配与 modules.enabled 开关）、`bff/admin`（审核摘要聚合，只读）
- **契约**：`contracts/openapi/mp.yaml`（`/ai/*`）、`admin.yaml`（`/admin/ai/*`）、`contracts/errors/`（ai 错误码段）；`ai` 不发布领域事件（附录 C 无 ai 事件）
- **前端代码**：`apps/miniapp/src/ai-entry/`、`features/{profile,content,discovery}` 嵌入点；`apps/admin-web/src/features/ai/`
- **测试**：`tests/ai`（评测样本集与门禁）、降级回归（关闭开关全链路）、archcheck、契约测试、权限矩阵补充
- **不动**：profile/content/discovery/trust 的既有手动路径与状态机；本阶段不改动这些模块的既有行为，仅新增对 `ai` contract 的可选调用
