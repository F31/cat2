package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/obs"
	"catcube/internal/modules/ai/contract"
)

// 后台管理（p5 §7.2）：提示词版本与用量/采纳率/成本。
// 路由由 ai 模块注册（Admin: true）；ai 未装载时路由不存在，不影响主链路（§9.1）。

// Prompts 提示词版本列表（后台）。
func (m *Module) Prompts(ctx context.Context) ([]contract.Prompt, error) {
	rows, err := m.deps.Pool.Query(ctx, `
		SELECT feature, version, body, approved FROM ai_prompt
		ORDER BY feature, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contract.Prompt{}
	for rows.Next() {
		var p contract.Prompt
		if err := rows.Scan(&p.Feature, &p.Version, &p.Body, &p.Approved); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SavePrompt 新增/更新提示词版本（approved 决定是否生效）。
func (m *Module) SavePrompt(ctx context.Context, feature, version, body string, approved bool) error {
	_, err := m.deps.Pool.Exec(ctx, `
		INSERT INTO ai_prompt (feature, version, body, approved) VALUES ($1,$2,$3,$4)
		ON CONFLICT (feature, version) DO UPDATE SET body=EXCLUDED.body, approved=EXCLUDED.approved`,
		feature, version, body, approved)
	return err
}

// Usage 近 days 天各能力用量/采纳率/成本。
func (m *Module) Usage(ctx context.Context, days int) ([]contract.Usage, error) {
	if days <= 0 {
		days = 7
	}
	rows, err := m.deps.Pool.Query(ctx, `
		SELECT feature,
		       count(*) FILTER (WHERE outcome='ok') AS calls,
		       count(*) FILTER (WHERE outcome='degraded') AS degraded,
		       count(*) FILTER (WHERE adopted = 1) AS adopted,
		       count(*) FILTER (WHERE adopted = 0) AS rejected,
		       COALESCE(sum(tokens),0) AS tokens
		FROM ai_call_log
		WHERE created_at >= now() - ($1 || ' days')::interval
		GROUP BY feature ORDER BY feature`, strconv.Itoa(days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	per1k := int64(0)
	if m.deps.Settings != nil {
		if v, err := m.deps.Settings.Int(ctx, 1, KeyCostPer1kTokens); err == nil && v > 0 {
			per1k = v
		}
	}
	out := []contract.Usage{}
	for rows.Next() {
		var u contract.Usage
		if err := rows.Scan(&u.Feature, &u.Calls, &u.Degraded, &u.Adopted, &u.Rejected, &u.Tokens); err != nil {
			return nil, err
		}
		fb := u.Adopted + u.Rejected
		if fb > 0 {
			u.AdoptRate = float64(u.Adopted) / float64(fb)
		}
		u.CostFen = u.Tokens * per1k / 1000
		out = append(out, u)
	}
	return out, rows.Err()
}

// handleAdminPrompts GET /api/admin/v1/ai/prompts
func (m *Module) handleAdminPrompts(w http.ResponseWriter, req *http.Request) {
	list, err := m.Prompts(req.Context())
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"items": list})
}

// handleAdminSavePrompt PUT /api/admin/v1/ai/prompts
func (m *Module) handleAdminSavePrompt(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Feature  string `json:"feature"`
		Version  string `json:"version"`
		Body     string `json:"body"`
		Approved bool   `json:"approved"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Feature == "" || body.Version == "" || body.Body == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.SavePrompt(req.Context(), body.Feature, body.Version, body.Body, body.Approved); err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	if m.deps.Audit != nil {
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin",
			Action: "ai:prompt:save", Result: audit.ResultOK, IP: audit.ClientIP(req),
			Detail: audit.Change{
				Before: map[string]any{},
				After:  map[string]any{"feature": body.Feature, "version": body.Version, "approved": body.Approved},
			}.Snapshot("提示词版本变更（评测门禁：仅 approved 生效）"),
		})
	}
	writeData(w, map[string]any{"saved": true})
}

// handleAdminUsage GET /api/admin/v1/ai/usage?days=7
func (m *Module) handleAdminUsage(w http.ResponseWriter, req *http.Request) {
	days := 7
	if v := req.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}
	list, err := m.Usage(req.Context(), days)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"days": days, "items": list})
}

// observeGuard 定时观测：采纳率骤降 / 成本异常告警（p5 §8.2；阈值经后台配置）。
func (m *Module) observeGuard(ctx context.Context) error {
	list, err := m.Usage(ctx, 1)
	if err != nil {
		return err
	}
	var calls, adopted, rejected, tokens, cost int64
	for _, u := range list {
		calls += u.Calls
		adopted += u.Adopted
		rejected += u.Rejected
		tokens += u.Tokens
		cost += u.CostFen
	}
	_ = calls
	obs.Default().SetGauge("catcube_ai_tokens_daily", float64(tokens))
	obs.Default().SetGauge("catcube_ai_cost_fen_daily", float64(cost))

	if m.deps.Settings != nil {
		if th, err := m.deps.Settings.Int(ctx, 1, KeyAlertCostFenDaily); err == nil && th > 0 && cost > th {
			obs.Default().Alert("ai_cost_anomaly", fmt.Sprintf("AI 当日成本 %d 分 > 阈值 %d 分", cost, th))
		}
	}

	fb := adopted + rejected
	minSample, minRate := int64(20), int64(30)
	if m.deps.Settings != nil {
		if v, err := m.deps.Settings.Int(ctx, 1, KeyAlertAdoptMinSample); err == nil && v > 0 {
			minSample = v
		}
		if v, err := m.deps.Settings.Int(ctx, 1, KeyAlertAdoptRateMin); err == nil && v > 0 {
			minRate = v
		}
	}
	if fb >= minSample {
		rate := adopted * 100 / fb
		obs.Default().SetGauge("catcube_ai_adopt_rate_pct", float64(rate))
		if rate < minRate {
			obs.Default().Alert("ai_adopt_rate_drop", fmt.Sprintf("AI 采纳率 %d%% < 阈值 %d%%（样本 %d）", rate, minRate, fb))
		}
	}
	return nil
}
