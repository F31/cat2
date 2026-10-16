# p5-ai-scenarios · 任务清单

> 前置：p0 待确认 #3（产品确认纳入一期）、#4（法务合规评估）已通过；未通过则本变更整体取消，ai 不装载。

## 1. ai 模块（R0）

- [x] 1.1 模块结构 `modules/ai`（R0，无同阶依赖：认证经 `AuthFunc` 注入）；迁移 `ai_call_log`（场景/用户/token/耗时/outcome/adopted/input_len）；`admin/` 页面归 p4
- [x] 1.2 端口与契约套件：`ports.LLM`（已有）+ 新增 `ports.OCR`/`ports.ASR`；开发态适配器 `llm.DevClient/DevOCR/DevASR`（含 `FAULT` 故障模拟）并接入 `ports/suite`（`TestLLMContract`/`TestOCRContract`/`TestASRContract`）；境内供应商适配器待 #4 结论后接入
- [x] 1.3 contract：`OnboardDraft`/`PublishDraft`（Draft）/`Search`（Translate2Query）/`Feedback`/`Enabled`；**降级签名**：开关关闭或上游失败统一返回 `030001`（HTTP 503），前端回退原表单
- [x] 1.4 提示词版本：`ai_prompt(feature,version,body,approved)`；生效规则=**仅 approved=true 的最高版本**，否则用代码默认 `code-v1`；`ai_call_log.prompt_version` 留痕；集成测试验证未评测版本不生效、评测通过即生效
- [x] 1.5 配额与成本：SettingsSchema `ai.quota.user.daily`/`ai.budget.fen.daily`/`ai.cost.fen_per_1ktoken`；按用户/日计数 + 预算用尽 → `030004`/429；成本指标 `catcube_ai_cost_fen_total`、超限计数 `catcube_ai_quota_exceeded_total`；**并修复配置三层缺口**：`pg.Settings` 存储接入 + 后台 `GET/PUT /api/admin/v1/settings` + admin-web「业务配置」页（热生效）
- [x] 1.6 关闭即降级：默认开关关闭（`ai.onboard`/`ai.publish`/`ai.search` 未登记即不可用），`--modules` 不含 ai 时路由不存在，调用方一律走原手动路径（`TestIntegrationAIFlagGateAndDegrade` 第 2 步验证原表单不受影响）
- [x] 1.7 隐私红线：`buildPrompt` 统一构造入口，剥离手机号/证件号/邮箱/微信号（`sanitizeForPrompt`）并计数 `catcube_ai_privacy_scrub_total`；单测 + 集成测试覆盖
- [x] 1.8 调用留痕：每次调用（含降级）记 `ai_call_log`，采纳经 `/ai/feedback` 回写；指标 `catcube_ai_call_total`/`catcube_ai_degraded_total`/`catcube_ai_adopted_total`/`catcube_ai_rejected_total`/`catcube_ai_tokens_total`

## 2. 场景一：一句话入驻

- [x] 2.1 入驻页 ai-entry：`pages/card/edit` 嵌入「一句话生成名片草稿」（文字输入；语音输入待接入 ASR 端口）→ `OnboardDraft` 预填名称/简介/优势/背景；开关关闭时入口不渲染（纯手工）
- [x] 2.2 输出收敛：草稿字段经字段池白名单丢弃未登记项（`catcube_ai_output_dropped_total`）；检索条件 industry/need/offer/region 经 `dict.Resolve` 校验，非法丢弃；类型幻觉经 `validSearchType` 丢弃
- [x] 2.3 人在回路：AI 仅返回草稿（不落库），写入仍走原接口；`/ai/feedback` 记录采纳/放弃并要求「用户确认后才写入」

## 3. 场景二：发布智能草稿

- [x] 3.1 发布页 ai-entry：`pages/content/publish` 按当前 Kind 调 `PublishDraft` → 预填标题与字段；仅预填，提交仍走原校验与内容安全（提交成功后 `aiFeedback(adopted=true)` 记采纳）
- [x] 3.2 结构化输出校验：JSON 解析失败即降级（`030001` 走手动填写）；字段收敛按字段池白名单（同 2.2）；提交仍走原校验与内容安全流水线
- [x] 3.3 草稿仅为预填，提交仍走原有校验与内容安全流水线（`TestIntegrationAIPublishDraftPrefillOnly`：草稿不落库、原接口提交成功、缺必填仍 400）

## 4. 场景三：自然语言搜索

- [x] 4.1 搜索页接入：`pages/search` 「AI 智能搜索」→ `Search` 解析出类型/区域/行业/需求/供给/关键词并直接检索；解析失败经 `030001` 回退关键词搜索并提示
- [x] 4.2 回退：`Search` 不可用返回 `030001`，前端回退普通关键词搜索（`/search`）；类型幻觉经 `validSearchType` 丢弃

## 5. 场景四：智能客服（经产品确认）

- [x] 5.1 我的页新增「客服」入口 → `pages/support`：人工客服电话与工作时间 9:00-18:00 常驻；智能客服按 `ai.cs` 开关展示（关闭时仅人工信息），入口不影响人工客服
- [x] 5.2 知识范围限定平台规则与操作指引：`CustomerService` 提示词与 dev 规则仅答平台主题，越界/不确定一律 `escalated=true` 并给转人工话术（`escalateHint`）；上游故障/关闭 → `030001` 前端提示人工；评测 `TestAIEvalCustomerServiceScope` + 集成 `TestIntegrationAICustomerService`

## 6. 审核辅助

- [x] 6.1 审核辅助：`ai.ReviewSuggestion`（预分类 fraud/ad/abuse/porn/other + 风险 0-3 + 摘要，开关 `ai.review`）；trust 经装配根桥接（同阶不互相依赖）暴露 `GET /api/admin/v1/review-tasks/{id}/suggestion`；**只读建议**，未启用/上游故障/配额用尽一律 `available=false`，审核流程不受影响；admin-web 审核页「AI 建议」；集成测试 `TestIntegrationAIReviewSuggestion`
- [x] 6.2 处罚结论必须人工确认：AI 仅返回只读建议；`TestIntegrationAIReviewSuggestion` 断言取得建议后任务仍「待审」（无状态机转移），复核仅由人工 `/review-tasks/{id}/resolve` 触发

## 7. 前端

- [x] 7.1 `apps/miniapp/src/ai-entry/` 通用组件：嵌入入驻页（`pages/card/edit`）/发布页（`pages/content/publish`）/搜索页（`pages/search`）；开关关闭或 `ai` 未装载时自动隐藏（`aiFeatures` 评估），统一空输入校验/加载态/不可用提示，不影响手动填写
- [x] 7.2 admin-web AI 管理页（`pages/AiPrompts.tsx`）：提示词版本（新增/编辑，approved=生效门禁）+ 近 7 天用量表（调用/降级/采纳/采纳率/tokens/成本）；服务端 `GET/PUT /api/admin/v1/ai/prompts`、`GET /api/admin/v1/ai/usage`（审计 `ai:prompt:save`），见 `TestIntegrationAIAdminPromptsUsage`

## 8. 评测与观测

- [x] 8.1 固定样本集评测（CI 门禁 `go test ./...`）：`internal/modules/ai/eval_test.go`（`TestAIEvalSuite` 抽取准确率≥0.9 + 降级路径 100%、`TestAIEvalTagSetLegality` 类型/分类合法率≥0.9、`TestAIEvalPrivacy`）；提示词/收敛逻辑变更导致指标下降即失败
- [x] 8.2 采纳率与成本观测：`GET /api/admin/v1/ai/usage` + admin-web「AI 提示词与用量」页（看板）；定时任务 `ai.cost_guard`（`observeGuard`）按后台阈值 `ai.alert.cost.fen.daily`/`ai.alert.adopt_rate.min`/`ai.alert.adopt.min_sample` 触发 `ai_cost_anomaly`/`ai_adopt_rate_drop` 告警（`docs/ops/alerts.md`），`TestIntegrationAIAdminPromptsUsage` 覆盖

## 9. 退出标准

- [x] 9.1 不装载 ai 全链路无影响：`--modules`/`CATCUBE_MODULES` 排除 ai 时 AI 路由不存在（404），主链路照常（`TestIntegrationAIModuleExcluded`）
- [x] 9.2 评测指标达标：`TestAIEvalSuite`/`TestAIEvalTagSetLegality` 阈值（≥0.9）在固定样本集上通过
- [x] 9.3 采纳率与成本可观测：见 7.2/8.2（用量接口 + 管理页 + `ai.cost_guard` 告警）
