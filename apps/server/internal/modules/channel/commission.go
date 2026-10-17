// 佣金与结算（p4 §2）：计佣规则可配 + OrderPaid 计佣快照 + 月结状态机（复核≠执行）。
package channel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/settings"
	"catcube/internal/kernel/statemachine"
	"catcube/internal/modules/channel/domain"
	"catcube/pkg/ids"
)

// Schemas 渠道配置（后台「系统设置」/渠道配置，热生效）。
var Schemas = []settings.Schema{
	{Key: "channel.commission.first_ratio_bp", Type: settings.TypeInt, Default: "3000", Description: "首单计佣比例（万分比，3000=30%）"},
	{Key: "channel.commission.renew_ratio_bp", Type: settings.TypeInt, Default: "1000", Description: "续费计佣比例（万分比，1000=10%）"},
	{Key: "channel.commission.scope", Type: settings.TypeString, Default: "membership", Description: "计佣业务范围（逗号分隔 biz_type；不含立方分摊与保证金）"},
	{Key: "channel.agency_fee.first_year_fen", Type: settings.TypeFen, Default: "3000000", Description: "代理费首年（分）"},
	{Key: "channel.agency_fee.renew_year_fen", Type: settings.TypeFen, Default: "2000000", Description: "代理费次年（分）"},
	{Key: "channel.guarantee.fen", Type: settings.TypeFen, Default: "1000000", Description: "保证金（分，可退预收台账）"},
}

func (m *Module) settingInt(ctx context.Context, key string, def int64) int64 {
	if m.deps.Settings == nil {
		return def
	}
	if v, err := m.deps.Settings.Int(ctx, 1, key); err == nil {
		return v
	}
	return def
}

func (m *Module) settingStr(ctx context.Context, key, def string) string {
	if m.deps.Settings == nil {
		return def
	}
	if v, err := m.deps.Settings.Get(ctx, 1, key); err == nil && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

// accrueOnOrderPaid 订阅制订单支付成功 → 计佣（首单/续费按配置比例，写快照，幂等由事件去重保证）。
func (m *Module) accrueOnOrderPaid(ctx context.Context, e eventbus.Event) error {
	var p struct {
		OrderID   int64  `json:"order_id,string"`
		BizType   string `json:"biz_type"`
		BuyerID   int64  `json:"buyer_id,string"`
		AmountFen int64  `json:"amount_fen"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return nil
	}
	if p.AmountFen <= 0 || p.BuyerID == 0 {
		return nil
	}
	// 计佣范围（不含立方分摊与保证金）
	inScope := false
	for _, s := range strings.Split(m.settingStr(ctx, "channel.commission.scope", "membership"), ",") {
		if strings.TrimSpace(s) == p.BizType {
			inScope = true
		}
	}
	if !inScope {
		return nil
	}
	// 归属运营商
	var operatorID int64
	if err := m.deps.Pool.QueryRow(ctx,
		`SELECT operator_id FROM chn_attribution WHERE user_id=$1 AND unbind_at IS NULL`, p.BuyerID).Scan(&operatorID); err != nil {
		return nil // 非归属用户不计佣
	}
	// 首单/续费：无历史佣金即首单
	var prior int
	_ = m.deps.Pool.QueryRow(ctx, `SELECT count(1) FROM chn_commission WHERE user_id=$1`, p.BuyerID).Scan(&prior)
	kind := "first"
	ratio := m.settingInt(ctx, "channel.commission.first_ratio_bp", 3000)
	if prior > 0 {
		kind = "renew"
		ratio = m.settingInt(ctx, "channel.commission.renew_ratio_bp", 1000)
	}
	amount := p.AmountFen * ratio / 10000
	month := time.Now().Format("2006-01")
	snap, _ := json.Marshal(map[string]any{
		"ratio_bp": ratio, "kind": kind, "scope": p.BizType, "base_fen": p.AmountFen,
	})
	id := int64(ids.New())
	if _, err := m.deps.Pool.Exec(ctx, `
		INSERT INTO chn_commission (id, operator_id, user_id, order_id, biz_type, base_fen, ratio_bp, ratio_snapshot, amount_fen, month, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,0)`,
		id, operatorID, p.BuyerID, p.OrderID, p.BizType, p.AmountFen, int(ratio), snap, amount, month); err != nil {
		return err
	}
	// 保底任务：付费会员累计
	if _, err := m.deps.Pool.Exec(ctx, `
		INSERT INTO chn_annual_target (operator_id, year, paid_member_count)
		VALUES ($1,$2,1)
		ON CONFLICT (operator_id, year) DO UPDATE SET paid_member_count = chn_annual_target.paid_member_count + 1, updated_at=now()`,
		operatorID, time.Now().Year()); err != nil {
		return err
	}
	if m.deps.Bus != nil {
		payload, _ := json.Marshal(map[string]any{
			"operator_id": operatorID, "user_id": p.BuyerID, "order_id": p.OrderID,
			"amount_fen": amount, "month": month,
		})
		_ = m.deps.Bus.Publish(ctx, nil, eventbus.Event{
			ID: ids.New().String(), Topic: "channel.CommissionAccrued.v1", Payload: payload, SpaceID: 1,
		})
	}
	return nil
}

// generateSettlement 生成/复用某运营商某月结算单（汇总待入账佣金）。
func (m *Module) generateSettlement(ctx context.Context, operatorID int64, month string) (int64, error) {
	if operatorID == 0 {
		return 0, errBadInput
	}
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	var sum int64
	if err := m.deps.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_fen),0) FROM chn_commission
		WHERE operator_id=$1 AND month=$2 AND status=0`, operatorID, month).Scan(&sum); err != nil {
		return 0, err
	}
	var id int64
	if err := m.deps.Pool.QueryRow(ctx, `SELECT id FROM chn_settlement WHERE operator_id=$1 AND month=$2`, operatorID, month).Scan(&id); err == nil {
		if _, err := m.deps.Pool.Exec(ctx, `
			UPDATE chn_settlement SET commission_fen=$2, payable_fen=$2+adjust_fen WHERE id=$1`, id, sum); err != nil {
			return 0, err
		}
		return id, nil
	}
	id = int64(ids.New())
	if _, err := m.deps.Pool.Exec(ctx, `
		INSERT INTO chn_settlement (id, operator_id, month, commission_fen, payable_fen, status)
		VALUES ($1,$2,$3,$4,$4,0)`, id, operatorID, month, sum); err != nil {
		return 0, err
	}
	return id, nil
}

func (m *Module) settlementStatus(ctx context.Context, id int64) (int16, int64, error) {
	var status int16
	var reviewedBy int64
	err := m.deps.Pool.QueryRow(ctx, `SELECT status, reviewed_by FROM chn_settlement WHERE id=$1`, id).Scan(&status, &reviewedBy)
	return status, reviewedBy, err
}

func (m *Module) settlementTransition(ctx context.Context, id int64, from int16, event string) (int16, error) {
	to, err := statemachine.NewEngine(nil).Apply(ctx, domain.MachineSettlement, id, strconv.Itoa(int(from)), event, "*")
	if err != nil {
		return 0, errInvalidState
	}
	n, _ := strconv.Atoi(to)
	return int16(n), nil
}

// ReviewSettlement 复核（待结算→已复核）。
func (m *Module) ReviewSettlement(ctx context.Context, id, actorID int64) error {
	status, _, err := m.settlementStatus(ctx, id)
	if err != nil {
		return errNoOperator
	}
	to, err := m.settlementTransition(ctx, id, status, "review")
	if err != nil {
		return err
	}
	_, err = m.deps.Pool.Exec(ctx, `UPDATE chn_settlement SET status=$2, reviewed_by=$3 WHERE id=$1`, id, to, actorID)
	return err
}

// RejectSettlement 驳回（待结算→已驳回）。
func (m *Module) RejectSettlement(ctx context.Context, id int64, reason string) error {
	status, _, err := m.settlementStatus(ctx, id)
	if err != nil {
		return errNoOperator
	}
	to, err := m.settlementTransition(ctx, id, status, "reject")
	if err != nil {
		return err
	}
	_, err = m.deps.Pool.Exec(ctx, `UPDATE chn_settlement SET status=$2, reason=$3 WHERE id=$1`, id, to, reason)
	return err
}

// PaySettlement 打款（已复核→已打款；复核≠执行由内核 policy 强制）。
func (m *Module) PaySettlement(ctx context.Context, id, actorID int64, voucherRef string) error {
	status, reviewedBy, err := m.settlementStatus(ctx, id)
	if err != nil {
		return errNoOperator
	}
	if m.deps.Policy != nil {
		if err := m.deps.Policy.EnforceDualControl(ctx, "channel:settlement", actorID, reviewedBy, 0); err != nil {
			return errDualControl
		}
	}
	to, err := m.settlementTransition(ctx, id, status, "pay")
	if err != nil {
		return err
	}
	if _, err := m.deps.Pool.Exec(ctx, `
		UPDATE chn_settlement SET status=$2, paid_by=$3, paid_at=now(), voucher_ref=$4 WHERE id=$1`,
		id, to, actorID, voucherRef); err != nil {
		return err
	}
	if m.deps.Bus != nil {
		payload, _ := json.Marshal(map[string]any{"settlement_id": id, "paid_by": actorID})
		_ = m.deps.Bus.Publish(ctx, nil, eventbus.Event{
			ID: ids.New().String(), Topic: "channel.SettlementPaid.v1", Payload: payload, SpaceID: 1,
		})
	}
	return nil
}

// ---- HTTP（结算/提现）----

func (m *Module) handleWithdraw(w http.ResponseWriter, req *http.Request) {
	op, ok, _ := m.operatorOf(req.Context(), userIDFrom(req.Context()))
	if !ok || op.Status != 1 {
		writeErr(w, codeNoOperator, "非已开通运营商")
		return
	}
	id, err := m.generateSettlement(req.Context(), op.ID, "")
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"settlement_id": strconv.FormatInt(id, 10)})
}

func (m *Module) handleListSettlements(w http.ResponseWriter, req *http.Request) {
	status := int16(-1)
	if v := req.URL.Query().Get("status"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			status = int16(n)
		}
	}
	q := `SELECT id, operator_id, month, commission_fen, adjust_fen, payable_fen, status, reviewed_by, paid_by, COALESCE(voucher_ref,'')
		FROM chn_settlement WHERE 1=1`
	args := []any{}
	if status >= 0 {
		args = append(args, status)
		q += ` AND status=$` + strconv.Itoa(len(args))
	}
	q += ` ORDER BY month DESC, id DESC LIMIT 100`
	rows, err := m.deps.Pool.Query(req.Context(), q, args...)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, operatorID, commissionFen, adjustFen, payableFen, reviewedBy, paidBy int64
		var month string
		var st int16
		var voucher string
		if err := rows.Scan(&id, &operatorID, &month, &commissionFen, &adjustFen, &payableFen, &st, &reviewedBy, &paidBy, &voucher); err != nil {
			writeErr(w, codeInternal, "服务错误")
			return
		}
		items = append(items, map[string]any{
			"id": id, "operator_id": operatorID, "month": month, "commission_fen": commissionFen,
			"adjust_fen": adjustFen, "payable_fen": payableFen, "status": st,
			"reviewed_by": reviewedBy, "paid_by": paidBy, "voucher_ref": voucher,
		})
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleGenerateSettlement(w http.ResponseWriter, req *http.Request) {
	var body struct {
		OperatorID string `json:"operator_id"`
		Month      string `json:"month"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	op, _ := strconv.ParseInt(body.OperatorID, 10, 64)
	id, err := m.generateSettlement(req.Context(), op, body.Month)
	if err != nil {
		writeChannelErr(w, err)
		return
	}
	writeData(w, map[string]any{"settlement_id": strconv.FormatInt(id, 10)})
}

func (m *Module) handleReviewSettlement(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	actor := userIDFrom(req.Context())
	if err := m.ReviewSettlement(req.Context(), id, actor); err != nil {
		m.auditAdmin(req, "channel:settlement:review", id, err, audit.ResultDenied)
		writeChannelErr(w, err)
		return
	}
	m.auditAdmin(req, "channel:settlement:review", id, nil, audit.ResultOK)
	writeData(w, map[string]any{"ok": true})
}

func (m *Module) handleRejectSettlement(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	if err := m.RejectSettlement(req.Context(), id, body.Reason); err != nil {
		m.auditAdmin(req, "channel:settlement:reject", id, err, audit.ResultDenied)
		writeChannelErr(w, err)
		return
	}
	m.auditAdmin(req, "channel:settlement:reject", id, nil, audit.ResultOK)
	writeData(w, map[string]any{"ok": true})
}

func (m *Module) handlePaySettlement(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		VoucherRef string `json:"voucher_ref"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	actor := userIDFrom(req.Context())
	if err := m.PaySettlement(req.Context(), id, actor, body.VoucherRef); err != nil {
		m.auditAdmin(req, "channel:settlement:pay", id, err, audit.ResultDenied)
		writeChannelErr(w, err)
		return
	}
	m.auditAdmin(req, "channel:settlement:pay", id, nil, audit.ResultOK)
	writeData(w, map[string]any{"ok": true})
}

var errDualControl = errors.New("channel: 复核≠执行（同一人不得复核并打款）")
