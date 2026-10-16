// Package ai R0 AI 网关模块（p5 一期框架）：一句话入驻/发布草稿/自然语言搜索。
// 设计要点：
//   - 全部能力受特性开关门禁（ai.onboard/ai.publish/ai.search），默认关闭；
//     开关关闭或上游（LLM）失败一律返回 030001，前端回退原表单，主链路不受影响（p6 §4.1）。
//   - 调用留痕 ai_call_log：用于采纳率与成本（tokens）观测；采纳/拒绝由 Feedback 更新。
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/featureflag"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/obs"
	"catcube/internal/kernel/settings"
	"catcube/internal/modules/ai/contract"
	"catcube/internal/ports"
	dictcontract "catcube/internal/shared/dict/contract"
	"catcube/pkg/ids"
)

// ai 错误码（contracts/errors/README.md 03 段）。
const (
	codeParamInvalid = 1     // 000001
	codeUnauthorized = 2     // 000002
	codeInternal     = 9     // 000009
	codeUnavailable  = 30001 // 030001 AI 能力不可用
	codeBadInput     = 30002 // 030002 输入非法或超长
	codeBadFeedback  = 30003 // 030003 采纳反馈目标非法
	codeQuotaUsed    = 30004 // 030004 AI 今日额度/预算已用尽
)

const maxInputRunes = 500

// AuthFunc 认证函数（装配根注入，避免 R0 同阶依赖 identity）。
type AuthFunc func(ctx context.Context, accessToken string) (userID int64, err error)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Flags    *featureflag.Store
	LLM      ports.LLM
	Auth     AuthFunc
	Settings *settings.Registry
	Dict     dictcontract.Facade
}

// Module ai 模块。
type Module struct {
	deps Deps
	dict dictcontract.Facade
}

func New(deps Deps) *Module { return &Module{deps: deps, dict: deps.Dict} }

func (m *Module) Name() string      { return "ai" }
func (m *Module) Rank() module.Rank { return module.RankR0 }

func (m *Module) DependsOn() []string { return nil }

const migrations = `
CREATE TABLE IF NOT EXISTS ai_call_log (
	id bigint PRIMARY KEY,
	prompt_version varchar(32) NOT NULL DEFAULT 'code-v1',
	feature varchar(32) NOT NULL,          -- onboard/publish/search
	user_id bigint NOT NULL,
	tokens int NOT NULL DEFAULT 0,
	latency_ms int NOT NULL DEFAULT 0,
	outcome varchar(16) NOT NULL,          -- ok/degraded
	adopted smallint,                       -- 0未采纳/1采纳/NULL未知
	input_len int NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now()
);
-- 兼容既有库：CREATE 之后补列
ALTER TABLE ai_call_log ADD COLUMN IF NOT EXISTS prompt_version varchar(32) NOT NULL DEFAULT 'code-v1';
CREATE INDEX IF NOT EXISTS idx_ai_call_user ON ai_call_log(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_call_feature ON ai_call_log(feature, outcome, created_at DESC);

CREATE TABLE IF NOT EXISTS ai_prompt (
	feature varchar(32) NOT NULL,
	version varchar(32) NOT NULL,
	body text NOT NULL,
	approved boolean NOT NULL DEFAULT false,  -- 评测通过后才能生效（p5 §1.4）
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (feature, version)
);
`

func (m *Module) Migrations() string { return migrations }

// Schemas AI 配额与成本配置（后台可配）。
var Schemas = []settings.Schema{
	{Key: KeyQuotaPerDay, Type: settings.TypeInt, Default: "50", Description: "单用户每日 AI 调用上限"},
	{Key: KeyBudgetPerDayFen, Type: settings.TypeFen, Default: "0", Description: "每日 AI 成本预算（分，0=不限）"},
	{Key: KeyCostPer1kTokens, Type: settings.TypeFen, Default: "0", Description: "每千 token 成本（分，用于预算估算）"},
}

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

func (m *Module) Register(r *module.Registry) error {
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/ai/onboard-draft", Handler: m.auth(m.handleOnboard)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/ai/publish-draft", Handler: m.auth(m.handlePublish)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/ai/search", Handler: m.auth(m.handleSearch)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/ai/feedback", Handler: m.auth(m.handleFeedback)},
		// 能力开关清单（前端据此决定是否展示 AI 入口）
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/ai/features", Handler: m.auth(m.handleFeatures)},
	)
	r.Settings = append(r.Settings, Schemas...)
	r.AdminSections = append(r.AdminSections,
		module.AdminSection{Key: "ai.calls", Label: "AI 调用与采纳", Path: "/ai-calls", Module: "ai"})
	return nil
}

// ---- 契约实现 ----

// Enabled 能力是否开启（特性开关 + 按用户灰度评估）。
func (m *Module) Enabled(ctx context.Context, userID int64, feature string) bool {
	if m.deps.Flags == nil || m.deps.LLM == nil {
		return false
	}
	f, ok, err := m.deps.Flags.Get(ctx, feature)
	if err != nil || !ok {
		return false
	}
	return featureflag.Eval(f, userID)
}

// OnboardDraft 一句话入驻 → 名片草稿。
func (m *Module) OnboardDraft(ctx context.Context, userID int64, text string) (contract.Draft, error) {
	return m.draft(ctx, userID, contract.FlagOnboard, "onboard", "", text)
}

// PublishDraft 一句话发布 → 内容草稿。
func (m *Module) PublishDraft(ctx context.Context, userID int64, kind, text string) (contract.Draft, error) {
	if !validKind(kind) {
		return contract.Draft{}, errBadInput
	}
	return m.draft(ctx, userID, contract.FlagPublish, "publish", kind, text)
}

// draft 统一草稿生成：配额/成本 → 开关 → 提示词（隐私过滤）→ LLM → 字段白名单收敛。
func (m *Module) draft(ctx context.Context, userID int64, flag, feature, kind, input string) (contract.Draft, error) {
	if strings.TrimSpace(input) == "" || len([]rune(input)) > maxInputRunes {
		return contract.Draft{}, errBadInput
	}
	if !m.Enabled(ctx, userID, flag) {
		return contract.Draft{}, errUnavailable
	}
	if err := m.checkQuota(ctx, userID); err != nil {
		return contract.Draft{}, err
	}
	prompt, version, scrubbed := m.buildFeaturePrompt(ctx, feature, kind, input)
	start := time.Now()
	raw, tokens, err := m.callLLM(ctx, prompt)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		m.logCall(ctx, feature, userID, 0, latency, "degraded", len([]rune(input)), version, scrubbed)
		return contract.Draft{}, errUnavailable
	}
	var parsed struct {
		Title  string              `json:"title"`
		Fields []map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		m.logCall(ctx, feature, userID, 0, latency, "degraded", len([]rune(input)), version, scrubbed)
		return contract.Draft{}, errUnavailable
	}
	fields := make([]contract.Field, 0, len(parsed.Fields))
	for _, f := range parsed.Fields {
		fields = append(fields, contract.Field{Field: f["field"], Value: f["value"]})
	}
	// 输出收敛：丢弃未登记字段（p5 §2.2/§3.2）
	allowed := m.fieldAllowed(ctx)
	fields, dropped := dropUnknownFields(fields, allowed)
	if dropped > 0 {
		obs.Default().Add("catcube_ai_output_dropped_total", float64(dropped))
	}
	callID := int64(ids.New())
	m.logCallWithID(ctx, callID, feature, userID, tokens, latency, "ok", len([]rune(input)), version, scrubbed)
	return contract.Draft{CallID: callID, Title: parsed.Title, Fields: fields, Tokens: tokens}, nil
}

// buildFeaturePrompt 取生效提示词（DB 已评测版本优先，否则代码默认）并做隐私过滤。
func (m *Module) buildFeaturePrompt(ctx context.Context, feature, kind, input string) (string, string, int) {
	body, version := m.activePrompt(ctx, feature)
	clean, removed := sanitizeForPrompt(input)
	if removed > 0 {
		obs.Default().Add("catcube_ai_privacy_scrub_total", float64(removed))
	}
	prompt := body
	if kind != "" {
		prompt += "（kind=" + kind + "）"
	}
	prompt += "\n文本:" + clean + "\n"
	return prompt, version, removed
}

// activePrompt 生效提示词：DB 中 approved=true 的最高版本；无则代码默认（code-v1）。
func (m *Module) activePrompt(ctx context.Context, feature string) (string, string) {
	def := defaultPrompt(feature)
	var body, version string
	err := m.deps.Pool.QueryRow(ctx, `
		SELECT body, version FROM ai_prompt WHERE feature=$1 AND approved=true
		ORDER BY version DESC LIMIT 1`, feature).Scan(&body, &version)
	if err != nil || body == "" {
		return def, "code-v1"
	}
	return body, version
}

// defaultPrompt 代码内默认提示词（首版；变更须经评测并在 ai_prompt 标记 approved）。
func defaultPrompt(feature string) string {
	switch feature {
	case "onboard":
		return "请把下面文本整理为名片草稿（onboard），输出 JSON：{title, fields:[{field,value}]}"
	case "search":
		return "请解析为检索条件（search），输出 JSON：{keyword,type,region,industry,need,offer}"
	default:
		return "请把下面文本整理为内容草稿（publish），输出 JSON：{title, fields:[{field,value}]}"
	}
}

// checkQuota 每日调用额度与成本预算（p5 §1.5）。
func (m *Module) checkQuota(ctx context.Context, userID int64) error {
	limit := int64(0)
	if m.deps.Settings != nil {
		if v, err := m.deps.Settings.Int(ctx, 1, KeyQuotaPerDay); err == nil {
			limit = v
		}
	}
	var calls, tokens int64
	if err := m.deps.Pool.QueryRow(ctx, `
		SELECT count(1), COALESCE(sum(tokens),0) FROM ai_call_log
		WHERE user_id=$1 AND created_at >= date_trunc('day', now())`, userID).Scan(&calls, &tokens); err != nil {
		return err
	}
	if limit > 0 && calls >= limit {
		obs.Default().Inc("catcube_ai_quota_exceeded_total")
		return errQuotaUsed
	}
	if m.deps.Settings != nil {
		budget, _ := m.deps.Settings.Int(ctx, 1, KeyBudgetPerDayFen)
		rate, _ := m.deps.Settings.Int(ctx, 1, KeyCostPer1kTokens)
		if budget > 0 && rate > 0 {
			costFen := tokens * rate / 1000
			if costFen >= budget {
				obs.Default().Inc("catcube_ai_quota_exceeded_total")
				return errQuotaUsed
			}
		}
	}
	return nil
}

// Search 自然语言搜索 → 结构化条件。
func (m *Module) Search(ctx context.Context, userID int64, text string) (contract.SearchQuery, error) {
	if strings.TrimSpace(text) == "" || len([]rune(text)) > maxInputRunes {
		return contract.SearchQuery{}, errBadInput
	}
	if !m.Enabled(ctx, userID, contract.FlagSearch) {
		return contract.SearchQuery{}, errUnavailable
	}
	if err := m.checkQuota(ctx, userID); err != nil {
		return contract.SearchQuery{}, err
	}
	prompt, version, scrubbed := m.buildFeaturePrompt(ctx, "search", "", text)
	start := time.Now()
	raw, tokens, err := m.callLLM(ctx, prompt)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		m.logCall(ctx, "search", userID, 0, latency, "degraded", len([]rune(text)), version, scrubbed)
		return contract.SearchQuery{}, errUnavailable
	}
	var q contract.SearchQuery
	if err := json.Unmarshal(raw, &q); err != nil {
		m.logCall(ctx, "search", userID, 0, latency, "degraded", len([]rune(text)), version, scrubbed)
		return contract.SearchQuery{}, errUnavailable
	}
	// 只保留平台登记的检索类型与标签 code（防模型幻觉直达检索层）
	if q.Type != "" && !validSearchType(q.Type) {
		q.Type = ""
	}
	q.Industry = m.filterValidCodes(ctx, q.Industry)
	q.Need = m.filterValidCodes(ctx, q.Need)
	q.Offer = m.filterValidCodes(ctx, q.Offer)
	if q.Region != "" && m.dict != nil {
		if o, err := m.dict.Resolve(ctx, q.Region); err != nil || !o.Enabled {
			q.Region = ""
		}
	}
	callID := int64(ids.New())
	m.logCallWithID(ctx, callID, "search", userID, tokens, latency, "ok", len([]rune(text)), version, scrubbed)
	q.CallID = callID
	q.Tokens = tokens
	return q, nil
}

// Feedback 采纳反馈：更新调用留痕的采纳标记（采纳率观测）。
func (m *Module) Feedback(ctx context.Context, userID, callID int64, adopted bool) error {
	if callID == 0 {
		return errBadFeedback
	}
	tag, err := m.deps.Pool.Exec(ctx,
		`UPDATE ai_call_log SET adopted=$3 WHERE id=$1 AND user_id=$2`,
		callID, userID, boolToInt16(adopted))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errBadFeedback
	}
	if adopted {
		obs.Default().Inc("catcube_ai_adopted_total")
	} else {
		obs.Default().Inc("catcube_ai_rejected_total")
	}
	return nil
}

// ---- 内部 ----

var (
	errUnavailable = errors.New("ai: 能力不可用")
	errQuotaUsed   = errors.New("ai: 今日额度已用尽")
	errBadInput    = errors.New("ai: 输入非法")
	errBadFeedback = errors.New("ai: 反馈目标非法")
)

func (m *Module) callLLM(ctx context.Context, prompt string) ([]byte, int, error) {
	obs.Default().Inc("catcube_ai_call_total")
	if m.deps.LLM == nil {
		obs.Default().Inc("catcube_ai_degraded_total")
		return nil, 0, errUnavailable
	}
	raw, err := m.deps.LLM.Complete(ctx, prompt, nil)
	if err != nil {
		obs.Default().Inc("catcube_ai_degraded_total")
		return nil, 0, err
	}
	// 估算 token（一期以字符数近似，用于成本量级观测）
	tokens := len([]rune(prompt))/2 + len(raw)/4
	obs.Default().Add("catcube_ai_tokens_total", float64(tokens))
	return raw, tokens, nil
}

func (m *Module) logCall(ctx context.Context, feature string, userID int64, tokens, latency int, outcome string, inputLen int, promptVersion string, scrubbed int) {
	m.logCallWithID(ctx, int64(ids.New()), feature, userID, tokens, latency, outcome, inputLen, promptVersion, scrubbed)
}

func (m *Module) logCallWithID(ctx context.Context, id int64, feature string, userID int64, tokens, latency int, outcome string, inputLen int, promptVersion string, scrubbed int) {
	if _, err := m.deps.Pool.Exec(ctx, `
		INSERT INTO ai_call_log (id, feature, user_id, tokens, latency_ms, outcome, input_len, prompt_version, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())`,
		id, feature, userID, tokens, latency, outcome, inputLen, promptVersion); err != nil {
		obs.Default().Inc("catcube_ai_log_failed_total")
	}
	// 成本估算（分）：tokens × 单价/1000
	if m.deps.Settings != nil && tokens > 0 {
		if rate, err := m.deps.Settings.Int(ctx, 1, KeyCostPer1kTokens); err == nil && rate > 0 {
			obs.Default().Add("catcube_ai_cost_fen_total", float64(int64(tokens)*rate/1000))
		}
	}
}

func validKind(kind string) bool {
	switch kind {
	case "project", "product", "meeting", "event":
		return true
	}
	return false
}

func validSearchType(t string) bool {
	switch t {
	case "user", "org", "content.project", "content.product", "content.meeting", "content.event":
		return true
	}
	return false
}

func boolToInt16(b bool) int16 {
	if b {
		return 1
	}
	return 0
}

// ---- HTTP ----

func (m *Module) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			writeErr(w, codeUnauthorized, "未登录")
			return
		}
		uid, err := m.deps.Auth(req.Context(), token)
		if err != nil {
			writeErr(w, codeUnauthorized, "登录已失效")
			return
		}
		next(w, req.WithContext(context.WithValue(req.Context(), ctxUserID{}, uid)))
	}
}

type ctxUserID struct{}

func userIDFrom(ctx context.Context) int64 {
	if v, ok := ctx.Value(ctxUserID{}).(int64); ok {
		return v
	}
	return 0
}

func bearer(req *http.Request) string {
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func (m *Module) handleOnboard(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	d, err := m.OnboardDraft(req.Context(), userIDFrom(req.Context()), body.Text)
	if err != nil {
		writeAIErr(w, err)
		return
	}
	writeData(w, d)
}

func (m *Module) handlePublish(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Kind string `json:"kind"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	d, err := m.PublishDraft(req.Context(), userIDFrom(req.Context()), body.Kind, body.Text)
	if err != nil {
		writeAIErr(w, err)
		return
	}
	writeData(w, d)
}

func (m *Module) handleSearch(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	q, err := m.Search(req.Context(), userIDFrom(req.Context()), body.Text)
	if err != nil {
		writeAIErr(w, err)
		return
	}
	writeData(w, q)
}

func (m *Module) handleFeedback(w http.ResponseWriter, req *http.Request) {
	var body struct {
		CallID  string `json:"call_id"`
		Adopted bool   `json:"adopted"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	id, _ := strconv.ParseInt(body.CallID, 10, 64)
	if err := m.Feedback(req.Context(), userIDFrom(req.Context()), id, body.Adopted); err != nil {
		writeAIErr(w, err)
		return
	}
	writeData(w, map[string]any{"ok": true})
}

// handleFeatures 能力开关清单（按当前用户灰度评估）。
func (m *Module) handleFeatures(w http.ResponseWriter, req *http.Request) {
	uid := userIDFrom(req.Context())
	writeData(w, map[string]any{"features": map[string]bool{
		contract.FlagOnboard: m.Enabled(req.Context(), uid, contract.FlagOnboard),
		contract.FlagPublish: m.Enabled(req.Context(), uid, contract.FlagPublish),
		contract.FlagSearch:  m.Enabled(req.Context(), uid, contract.FlagSearch),
	}})
}

func writeAIErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUnavailable):
		writeErr(w, codeUnavailable, "AI 暂不可用，请手动填写")
	case errors.Is(err, errBadInput):
		writeErr(w, codeBadInput, "内容过长或为空")
	case errors.Is(err, errBadFeedback):
		writeErr(w, codeBadFeedback, "反馈目标非法")
	case errors.Is(err, errQuotaUsed):
		writeErr(w, codeQuotaUsed, "AI 今日额度已用尽，请稍后再试或手动填写")
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus(code))
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": msg})
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeBadInput, codeBadFeedback, codeParamInvalid:
		return http.StatusBadRequest
	case codeUnavailable:
		return http.StatusServiceUnavailable
	case codeQuotaUsed:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}
