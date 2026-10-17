// 渠道后台：代理费/保证金台账、归属纠错、保底任务监控（p4 §2.1/2.5/2.6）。
package channel

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/obs"
	"catcube/pkg/ids"
)

// 保底任务阈值（12 个月）：≥50 付费会员 或 ≥3 落地项目。
const (
	targetPaidMembers     = 50
	targetLandingProjects = 3
)

// handleOperatorFees 运营商代理费/保证金台账。
func (m *Module) handleOperatorFees(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var agencyPaid int64
	var agencyYear int
	var guarantee int64
	if err := m.deps.Pool.QueryRow(ctxOr(req),
		`SELECT agency_fee_paid_fen, agency_fee_year, guarantee_fen FROM chn_operator WHERE id=$1`, id).
		Scan(&agencyPaid, &agencyYear, &guarantee); err != nil {
		writeChannelErr(w, errNoOperator)
		return
	}
	rows, err := m.deps.Pool.Query(ctxOr(req), `
		SELECT id, amount_fen, COALESCE(EXTRACT(EPOCH FROM paid_at)::bigint,0),
		       COALESCE(EXTRACT(EPOCH FROM refunded_at)::bigint,0), COALESCE(refund_reason,'')
		FROM chn_guarantee WHERE operator_id=$1 ORDER BY id DESC`, id)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var gid, amount, paidAt, refundedAt int64
		var reason string
		if err := rows.Scan(&gid, &amount, &paidAt, &refundedAt, &reason); err != nil {
			writeErr(w, codeInternal, "服务错误")
			return
		}
		items = append(items, map[string]any{
			"id": gid, "amount_fen": amount, "paid_at": paidAt, "refunded_at": refundedAt, "refund_reason": reason,
		})
	}
	writeData(w, map[string]any{
		"agency_fee_paid_fen": agencyPaid, "agency_fee_year": agencyYear,
		"guarantee_fen": guarantee, "guarantees": items,
	})
}

// handleRecordFees 代理费/保证金收款登记（金额可配，入 settings）。
func (m *Module) handleRecordFees(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		AgencyFeeFen int64 `json:"agency_fee_fen"`
		AgencyYear   int   `json:"agency_fee_year"`
		GuaranteeFen int64 `json:"guarantee_fen"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if body.AgencyFeeFen < 0 || body.GuaranteeFen < 0 {
		writeErr(w, codeParamInvalid, "金额非法")
		return
	}
	if _, err := m.deps.Pool.Exec(ctxOr(req), `
		UPDATE chn_operator SET agency_fee_paid_fen=$2, agency_fee_year=$3, guarantee_fen=$4, updated_at=now(), version=version+1
		WHERE id=$1`, id, body.AgencyFeeFen, body.AgencyYear, body.GuaranteeFen); err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	if body.GuaranteeFen > 0 {
		if _, err := m.deps.Pool.Exec(ctxOr(req), `
			INSERT INTO chn_guarantee (id, operator_id, amount_fen, paid_at) VALUES ($1,$2,$3,now())`,
			int64(ids.New()), id, body.GuaranteeFen); err != nil {
			writeErr(w, codeInternal, "服务错误")
			return
		}
	}
	m.auditAdmin(req, "channel:operator_fee:record", id, nil, audit.ResultOK)
	writeData(w, map[string]any{"ok": true})
}

// handleRefundGuarantee 保证金退款（走台账，不与收入混记）。
func (m *Module) handleRefundGuarantee(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	tag, err := m.deps.Pool.Exec(ctxOr(req), `
		UPDATE chn_guarantee SET refunded_at=now(), refund_reason=$2
		WHERE operator_id=$1 AND refunded_at IS NULL`, id, body.Reason)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, codeParamInvalid, "无可退保证金")
		return
	}
	if _, err := m.deps.Pool.Exec(ctxOr(req),
		`UPDATE chn_operator SET guarantee_fen=0, updated_at=now(), version=version+1 WHERE id=$1`, id); err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	m.auditAdmin(req, "channel:guarantee:refund", id, nil, audit.ResultOK)
	writeData(w, map[string]any{"ok": true})
}

// handleListAttributions 归属列表（后台）。
func (m *Module) handleListAttributions(w http.ResponseWriter, req *http.Request) {
	rows, err := m.deps.Pool.Query(ctxOr(req), `
		SELECT a.id, a.user_id, a.operator_id, a.region_code, a.source, EXTRACT(EPOCH FROM a.bound_at)::bigint
		FROM chn_attribution a WHERE a.unbind_at IS NULL
		ORDER BY a.bound_at DESC LIMIT 200`)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, uid, opID, boundAt int64
		var region string
		var source int16
		if err := rows.Scan(&id, &uid, &opID, &region, &source, &boundAt); err != nil {
			writeErr(w, codeInternal, "服务错误")
			return
		}
		items = append(items, map[string]any{
			"id": id, "user_id": uid, "operator_id": opID, "region_code": region, "source": source, "bound_at": boundAt,
		})
	}
	writeData(w, map[string]any{"items": items})
}

// handleCorrectAttribution 归属纠错（审批留痕，单一有效）。
func (m *Module) handleCorrectAttribution(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		OperatorID int64  `json:"operator_id"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.OperatorID == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	var userID int64
	var oldOperator int64
	if err := m.deps.Pool.QueryRow(ctxOr(req),
		`SELECT user_id, operator_id FROM chn_attribution WHERE id=$1 AND unbind_at IS NULL`, id).Scan(&userID, &oldOperator); err != nil {
		writeErr(w, codeParamInvalid, "归属不存在或已失效")
		return
	}
	var region string
	if err := m.deps.Pool.QueryRow(ctxOr(req), `SELECT region_code FROM chn_operator WHERE id=$1 AND status=1`, body.OperatorID).Scan(&region); err != nil {
		writeErr(w, codeParamInvalid, "目标运营商未开通")
		return
	}
	approval, _ := json.Marshal(map[string]any{
		"by": userIDFrom(req.Context()), "at": time.Now().Unix(), "from": oldOperator, "to": body.OperatorID, "reason": body.Reason,
	})
	if _, err := m.deps.Pool.Exec(ctxOr(req),
		`UPDATE chn_attribution SET unbind_at=now(), correct_approval=$2 WHERE id=$1`, id, approval); err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	if _, err := m.deps.Pool.Exec(ctxOr(req), `
		INSERT INTO chn_attribution (id, user_id, operator_id, region_code, source, bound_at, correct_approval)
		VALUES ($1,$2,$3,$4,3,now(),$5)`, int64(ids.New()), userID, body.OperatorID, region, approval); err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	m.auditAdmin(req, "channel:attribution:correct", id, nil, audit.ResultOK)
	writeData(w, map[string]any{"ok": true})
}

// countLandingProject 立方项目创建 → 按立方猫王归属运营商累加落地项目数（保底任务）。
func (m *Module) countLandingProject(ctx context.Context, e eventbus.Event) error {
	var p struct {
		CubeID int64 `json:"cube_id,string"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.CubeID == 0 {
		return nil
	}
	var leaderID int64
	if m.deps.Cube != nil {
		b, err := m.deps.Cube.Brief(ctx, p.CubeID)
		if err != nil {
			return nil
		}
		leaderID = b.LeaderID
	}
	if leaderID == 0 {
		return nil
	}
	var operatorID int64
	if err := m.deps.Pool.QueryRow(ctx,
		`SELECT operator_id FROM chn_attribution WHERE user_id=$1 AND unbind_at IS NULL`, leaderID).Scan(&operatorID); err != nil {
		return nil
	}
	_, err := m.deps.Pool.Exec(ctx, `
		INSERT INTO chn_annual_target (operator_id, year, landing_project_count)
		VALUES ($1,$2,1)
		ON CONFLICT (operator_id, year) DO UPDATE SET landing_project_count = chn_annual_target.landing_project_count + 1, updated_at=now()`,
		operatorID, time.Now().Year())
	return err
}

// monitorTargets 保底任务监控：更新 warn_flags 与指标（每日）。
func (m *Module) monitorTargets(ctx context.Context) error {
	rows, err := m.deps.Pool.Query(ctx, `
		SELECT t.operator_id, t.paid_member_count, t.landing_project_count
		FROM chn_annual_target t
		JOIN chn_operator o ON o.id=t.operator_id AND o.status=1
		WHERE t.year=$1`, time.Now().Year())
	if err != nil {
		return err
	}
	type row struct {
		id       int64
		members  int
		projects int
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.members, &r.projects); err != nil {
			rows.Close()
			return err
		}
		list = append(list, r)
	}
	rows.Close()
	warn := 0
	for _, r := range list {
		flags := map[string]any{}
		if r.members < targetPaidMembers && r.projects < targetLandingProjects {
			flags["members_low"] = true
			flags["projects_low"] = true
			warn++
		}
		raw, _ := json.Marshal(flags)
		if _, err := m.deps.Pool.Exec(ctx,
			`UPDATE chn_annual_target SET warn_flags=$2, updated_at=now() WHERE operator_id=$1 AND year=$3`,
			r.id, raw, time.Now().Year()); err != nil {
			return err
		}
	}
	obs.Default().SetGauge("catcube_channel_target_warn", float64(warn))
	return nil
}

// ctxOr 取请求上下文（后台 handler）。
func ctxOr(req *http.Request) context.Context { return req.Context() }
