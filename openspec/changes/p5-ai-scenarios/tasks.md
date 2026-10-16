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
- [ ] 3.3 草稿仅为预填，提交仍走原有校验与内容安全流水线

## 4. 场景三：自然语言搜索

- [x] 4.1 搜索页接入：`pages/search` 「AI 智能搜索」→ `Search` 解析出类型/区域/行业/需求/供给/关键词并直接检索；解析失败经 `030001` 回退关键词搜索并提示
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
