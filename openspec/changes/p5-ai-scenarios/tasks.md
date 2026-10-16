# p5-ai-scenarios · 任务清单

> 前置：p0 待确认 #3（产品确认纳入一期）、#4（法务合规评估）已通过；未通过则本变更整体取消，ai 不装载。

## 1. ai 模块（R0）

- [x] 1.1 模块结构 `modules/ai`（R0，无同阶依赖：认证经 `AuthFunc` 注入）；迁移 `ai_call_log`（场景/用户/token/耗时/outcome/adopted/input_len）；`admin/` 页面归 p4
- [ ] 1.2 `ports/llm`、`ports/ocr`、`ports/asr` 接口与供应商适配器（境内模型，依 #4 结论）；契约测试套件
- [x] 1.3 contract：`OnboardDraft`/`PublishDraft`（Draft）/`Search`（Translate2Query）/`Feedback`/`Enabled`；**降级签名**：开关关闭或上游失败统一返回 `030001`（HTTP 503），前端回退原表单
- [ ] 1.4 提示词版本管理：变更必须过评测门禁才能生效
- [ ] 1.5 配额与成本控制：QuotaDef（按用户/日）+ 预算 SettingsSchema；成本指标进看板
- [x] 1.6 关闭即降级：默认开关关闭（`ai.onboard`/`ai.publish`/`ai.search` 未登记即不可用），`--modules` 不含 ai 时路由不存在，调用方一律走原手动路径（`TestIntegrationAIFlagGateAndDegrade` 第 2 步验证原表单不受影响）
- [ ] 1.7 隐私红线：手机号/证件号/联系方式不进提示词与向量（构造提示词的统一入口做字段过滤，单测覆盖）
- [x] 1.8 调用留痕：每次调用（含降级）记 `ai_call_log`，采纳经 `/ai/feedback` 回写；指标 `catcube_ai_call_total`/`catcube_ai_degraded_total`/`catcube_ai_adopted_total`/`catcube_ai_rejected_total`/`catcube_ai_tokens_total`

## 2. 场景一：一句话入驻

- [ ] 2.1 入驻页嵌入 ai-entry：语音/文字输入 → `Extract` → 画像字段 + TagSet 草稿
- [ ] 2.2 输出收敛：TagSet 经 `dict.ValidateTagSet`，模型只能输出合法 code，非法丢弃并降级为手动填写
- [x] 2.3 人在回路：AI 仅返回草稿（不落库），写入仍走原接口；`/ai/feedback` 记录采纳/放弃并要求「用户确认后才写入」

## 3. 场景二：发布智能草稿

- [ ] 3.1 四类发布页嵌入 ai-entry：一句话 → `Draft` → 对应 ContentKind 字段草稿
- [ ] 3.2 结构化输出 + JSON Schema 校验（按 Kind 字段 schema）；不合规重试一次，仍不合规降级手动填写
- [ ] 3.3 草稿仅为预填，提交仍走原有校验与内容安全流水线

## 4. 场景三：自然语言搜索

- [ ] 4.1 搜索页接入 `Translate2Query`：自然语言 → 结构化查询（类型/TagSet/关键词）
- [x] 4.2 回退：`Search` 不可用返回 `030001`，前端回退普通关键词搜索（`/search`）；类型幻觉经 `validSearchType` 丢弃

## 5. 场景四：智能客服（经产品确认）

- [ ] 5.1 我的页客服入口接智能客服（工作时间 9:00-18:00 人工电话不变）
- [ ] 5.2 知识范围限定平台规则与操作指引；越界问题转人工话术

## 6. 审核辅助

- [ ] 6.1 trust 审核队列接入 AI 预分类与摘要（只读建议展示给审核员）
- [ ] 6.2 处罚结论必须人工确认（AI 输出不产生任何状态机转移）

## 7. 前端

- [ ] 7.1 `apps/miniapp/src/ai-entry/` 组件：嵌入入驻页/发布页/搜索页；`ai` 不可用或未装载时自动隐藏，不影响手动填写
- [ ] 7.2 admin-web `features/ai/`：提示词与用量管理页（版本、采纳率、成本）

## 8. 评测与观测

- [ ] 8.1 `tests/ai` 固定样本集评测：抽取准确率、TagSet 合法率、降级路径；提示词变更必须过评测（CI 门禁）
- [ ] 8.2 采纳率与成本指标进看板；告警：采纳率骤降、成本异常（阈值可配）

## 9. 退出标准

- [ ] 9.1 关闭开关（不装载 ai）时全链路回归测试无影响
- [ ] 9.2 评测指标达标（准确率/合法率阈值在评测集上达标）
- [ ] 9.3 采纳率与成本可观测（看板与告警上线）
