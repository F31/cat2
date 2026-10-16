package ops

import (
	"encoding/json"
	"net/http"
	"time"

	"catcube/internal/kernel/obs"
	"catcube/pkg/mask"
)

// telemetryEvents 允许入库的小程序事件（contracts/telemetry/README.md 登记）。
var telemetryEvents = map[string]bool{
	"register": true, "profile_complete": true, "feed_click": true, "search": true,
	"detail_view": true, "contact_initiate": true, "member_quote": true, "order_paid": true,
}

// telemetryPropWhitelist props 白名单键（防越权采集敏感信息）。
var telemetryPropWhitelist = map[string]bool{
	"channel": true, "tier": true, "ref": true, "type": true, "position": true,
	"keyword": true, "result_count": true, "result": true, "level": true, "years": true,
	"amount_fen": true, "order_id": true, "biz_type": true,
}

// handleTelemetry 批量上报（≤50 条/次；未登记事件丢弃；失败不阻断业务）。
func (m *Module) handleTelemetry(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Events []struct {
			Event      string         `json:"event"`
			OccurredAt string         `json:"occurred_at"`
			Props      map[string]any `json:"props"`
		} `json:"events"`
	}
	if err := decodeJSON(req, &body); err != nil {
		writeErr(w, 1, "参数错误")
		return
	}
	if len(body.Events) > 50 {
		body.Events = body.Events[:50]
	}
	uid := userIDFrom(req.Context())
	accepted := 0
	for _, e := range body.Events {
		if !telemetryEvents[e.Event] {
			obs.Default().Inc("catcube_telemetry_rejected_total")
			continue
		}
		props := filterProps(e.Props)
		at := time.Now()
		if e.OccurredAt != "" {
			if t, err := time.Parse(time.RFC3339, e.OccurredAt); err == nil {
				at = t
			}
		}
		if _, err := m.deps.Pool.Exec(req.Context(), `
			INSERT INTO tlm_event (user_id, event, props, occurred_at, created_at)
			VALUES ($1,$2,$3,$4,now())`, uid, e.Event, mustJSON(props), at); err != nil {
			obs.Default().Inc("catcube_telemetry_rejected_total")
			continue
		}
		accepted++
	}
	obs.Default().Add("catcube_telemetry_accepted_total", float64(accepted))
	writeData(w, map[string]any{"accepted": accepted})
}

func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

func containsPhone(s string) bool { return mask.ContainsPhoneLike(s) }
func containsID(s string) bool    { return mask.ContainsIDCardLike(s) }

// filterProps 仅保留白名单键，且剔除含手机号/证件号形态的值。
func filterProps(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if !telemetryPropWhitelist[k] {
			continue
		}
		if s, ok := v.(string); ok && (containsPhone(s) || containsID(s)) {
			continue
		}
		out[k] = v
	}
	return out
}

// handleDashboard 数据概览（12 项核心指标，口径见 docs/ops/dashboards.md）。
func (m *Module) handleDashboard(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	q1 := func(sql string) int64 {
		var n int64
		_ = m.deps.Pool.QueryRow(ctx, sql).Scan(&n)
		return n
	}
	metrics := map[string]any{
		// 账号
		"users_total":     q1(`SELECT count(1) FROM idn_user`),
		"users_new_today": q1(`SELECT count(1) FROM idn_user WHERE created_at >= date_trunc('day', now())`),
		"realname_users":  q1(`SELECT count(1) FROM idn_user WHERE realname_status=1`),
		// 内容与发现
		"contents_published":  q1(`SELECT count(1) FROM cnt_item WHERE status=1`),
		"search_events_today": q1(`SELECT count(1) FROM tlm_event WHERE event='search' AND occurred_at >= date_trunc('day', now())`),
		"feed_clicks_today":   q1(`SELECT count(1) FROM tlm_event WHERE event='feed_click' AND occurred_at >= date_trunc('day', now())`),
		// 联系与付费
		"contacts_today":    q1(`SELECT count(1) FROM tlm_event WHERE event='contact_initiate' AND occurred_at >= date_trunc('day', now())`),
		"members_active":    q1(`SELECT count(1) FROM mbr_member WHERE status=1 AND expires_at > now()`),
		"orders_paid_today": q1(`SELECT count(1) FROM trd_order WHERE status=1 AND paid_at >= date_trunc('day', now())`),
		"gmv_fen_today":     q1(`SELECT COALESCE(sum(amount_fen),0) FROM trd_order WHERE status=1 AND paid_at >= date_trunc('day', now())`),
		// 协作
		"cubes_active":   q1(`SELECT count(1) FROM cub_cube WHERE status=0`),
		"circles_active": q1(`SELECT count(1) FROM cir_circle WHERE status=0`),
		// 治理
		"review_backlog": q1(`SELECT count(1) FROM trs_review_task WHERE status=0`),
		"review_overdue": q1(`SELECT count(1) FROM trs_review_task WHERE status=0 AND sla_due_at < now()`),
		"outbox_dead":    q1(`SELECT count(1) FROM sys_outbox WHERE status=9`),
		// AI（可选）
		"ai_calls_today":    q1(`SELECT count(1) FROM ai_call_log WHERE created_at >= date_trunc('day', now())`),
		"ai_degraded_today": q1(`SELECT count(1) FROM ai_call_log WHERE outcome='degraded' AND created_at >= date_trunc('day', now())`),
	}
	writeData(w, map[string]any{"metrics": metrics, "note": "口径见 docs/ops/dashboards.md；实时查询，禁止为统计写业务表"})
}
