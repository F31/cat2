package ops

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/obs"
	"catcube/pkg/ids"
	"catcube/pkg/mask"
)

// 导出模板（p4 §7.4）：敏感字段白名单——不含明文联系方式/证件号。
var exportTemplates = map[string]string{
	"reconciliation": "对账导出（订单/支付/履约）",
	"orders":         "订单明细",
	"users":          "用户列表（脱敏）",
	"reviews":        "审核记录",
	"settlements":    "结算明细（渠道开通后启用）",
}

// exportParams 允许的时间范围参数。
type exportParams struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

// handleCreateExport 申请导出（异步生成；二次确认由前端负责，服务端全量审计）。
func (m *Module) handleCreateExport(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Template string `json:"template"`
		From     int64  `json:"from"`
		To       int64  `json:"to"`
	}
	if err := decodeJSON(req, &body); err != nil {
		writeErr(w, 1, "参数错误")
		return
	}
	if _, ok := exportTemplates[body.Template]; !ok {
		writeErr(w, 1, "未知导出模板")
		return
	}
	uid := userIDFrom(req.Context())
	id := int64(ids.New())
	if _, err := m.deps.Pool.Exec(req.Context(), `
		INSERT INTO sys_export_task (id, template, params, requester_id, status, created_at)
		VALUES ($1,$2,$3,$4,0,now())`,
		id, body.Template, fmt.Sprintf(`{"from":%d,"to":%d}`, body.From, body.To), uid); err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	// 导出审计（含模板与时间范围）
	if m.replayAudit() != nil {
		if aerr := m.replayAudit().Record(req.Context(), nil, audit.Entry{
			ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: "export:create",
			TargetRef: fmt.Sprintf(`{"id":"%d","type":"export"}`, id),
			Detail:    map[string]any{"template": body.Template, "from": body.From, "to": body.To},
			Result:    audit.ResultOK, IP: audit.ClientIP(req), RequestID: req.Header.Get("X-Request-ID"),
		}); aerr != nil {
			obs.Default().Inc("catcube_export_audit_failed_total")
			logErr("export audit", aerr)
		}
	}
	writeData(w, map[string]any{"id": strconv.FormatInt(id, 10), "status": 0})
}

// handleListExports 导出任务列表（本人；管理员可看全部）。
func (m *Module) handleListExports(w http.ResponseWriter, req *http.Request) {
	uid := userIDFrom(req.Context())
	all := req.URL.Query().Get("all") == "1"
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := m.deps.Pool.Query(req.Context(), `
		SELECT id, template, status, COALESCE(rows,0), COALESCE(err,''), created_at, finished_at, expires_at
		FROM sys_export_task
		WHERE ($1 OR requester_id=$2) ORDER BY created_at DESC LIMIT $3`, all, uid, limit)
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, rowsN int64
		var tpl, errMsg string
		var status int16
		var created time.Time
		var finished, expires *time.Time
		if err := rows.Scan(&id, &tpl, &status, &rowsN, &errMsg, &created, &finished, &expires); err != nil {
			writeErr(w, 9, "服务错误")
			return
		}
		items = append(items, map[string]any{
			"id": strconv.FormatInt(id, 10), "template": tpl, "status": status, "rows": rowsN,
			"error": errMsg, "created_at": created, "finished_at": finished, "expires_at": expires,
		})
	}
	writeData(w, map[string]any{"items": items})
}

// handleDownloadExport 下载导出文件（带水印）。
func (m *Module) handleDownloadExport(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var key string
	var status int16
	var requester int64
	if err := m.deps.Pool.QueryRow(req.Context(), `
		SELECT COALESCE(object_key,''), status, requester_id FROM sys_export_task WHERE id=$1`, id).
		Scan(&key, &status, &requester); err != nil {
		writeErr(w, 4, "导出任务不存在")
		return
	}
	if status != 2 || key == "" {
		writeErr(w, 400, "文件尚未生成")
		return
	}
	url, err := m.deps.Storage.URL(req.Context(), "default", key, "")
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	writeData(w, map[string]any{"url": url, "watermark": true})
}

// exportRunner 生成待处理导出（worker 定时任务）。
func (m *Module) exportRunner(ctx context.Context) error {
	rows, err := m.deps.Pool.Query(ctx, `
		SELECT id, template, params, requester_id FROM sys_export_task
		WHERE status=0 ORDER BY created_at LIMIT 5`)
	if err != nil {
		return err
	}
	type task struct {
		id          int64
		template    string
		from, to    int64
		requesterID int64
	}
	var tasks []task
	for rows.Next() {
		var t task
		var params []byte
		if err := rows.Scan(&t.id, &t.template, &params, &t.requesterID); err != nil {
			rows.Close()
			return err
		}
		var p exportParams
		_ = jsonUnmarshal(params, &p)
		t.from, t.to = p.From, p.To
		tasks = append(tasks, t)
	}
	rows.Close()
	for _, t := range tasks {
		if _, err := m.deps.Pool.Exec(ctx, `UPDATE sys_export_task SET status=1 WHERE id=$1`, t.id); err != nil {
			return err
		}
		body, n, gerr := m.renderExport(ctx, t.template, t.from, t.to, t.requesterID)
		if gerr != nil {
			_, _ = m.deps.Pool.Exec(ctx, `UPDATE sys_export_task SET status=3, err=$2, finished_at=now() WHERE id=$1`, t.id, gerr.Error())
			obs.Default().Inc("catcube_export_failed_total")
			continue
		}
		key := fmt.Sprintf("exports/%d.csv", t.id)
		if err := m.deps.Storage.Put(ctx, "default", key, bytes.NewReader(body)); err != nil {
			_, _ = m.deps.Pool.Exec(ctx, `UPDATE sys_export_task SET status=3, err=$2, finished_at=now() WHERE id=$1`, t.id, err.Error())
			obs.Default().Inc("catcube_export_failed_total")
			continue
		}
		wm := fmt.Sprintf("catcube export #%d requester=%d %s", t.id, t.requesterID, time.Now().UTC().Format(time.RFC3339))
		if _, err := m.deps.Pool.Exec(ctx, `
			UPDATE sys_export_task SET status=2, object_key=$2, rows=$3, watermark=$4,
				finished_at=now(), expires_at=now() + interval '7 days' WHERE id=$1`, t.id, key, n, wm); err != nil {
			return err
		}
		obs.Default().Add("catcube_export_rows_total", float64(n))
	}
	return nil
}

// renderExport 按模板生成 CSV（含水印行；敏感字段白名单）。
func (m *Module) renderExport(ctx context.Context, template string, from, to, requester int64) ([]byte, int, error) {
	if from == 0 {
		from = time.Now().AddDate(0, 0, -30).Unix()
	}
	if to == 0 {
		to = time.Now().Unix()
	}
	var buf bytes.Buffer
	buf.WriteString("# 水印：" + fmt.Sprintf("catcube export requester=%d %s", requester, time.Now().UTC().Format(time.RFC3339)) + "\n")
	cw := csv.NewWriter(&buf)
	n := 0
	switch template {
	case "orders", "reconciliation":
		cw.Write([]string{"order_id", "biz_type", "amount_fen", "status", "paid_at", "refund_fen", "fulfillment_status", "retry_count"})
		rows, err := m.deps.Pool.Query(ctx, `
			SELECT o.id, o.biz_type, o.amount_fen, o.status, o.paid_at,
			       COALESCE((SELECT sum(r.amount_fen) FROM trd_refund r
			                 JOIN trd_payment p ON p.id=r.payment_id
			                 WHERE p.order_id=o.id AND r.status=1),0),
			       COALESCE(f.status,-1), COALESCE(f.retry_count,0)
			FROM trd_order o LEFT JOIN trd_fulfillment f ON f.order_id=o.id
			WHERE o.created_at >= to_timestamp($1) AND o.created_at < to_timestamp($2)
			ORDER BY o.created_at`, from, to)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		for rows.Next() {
			var id, amount, refund int64
			var biz string
			var status, fstatus int16
			var paid *time.Time
			var retry int
			if err := rows.Scan(&id, &biz, &amount, &status, &paid, &refund, &fstatus, &retry); err != nil {
				return nil, 0, err
			}
			var paidStr string
			if paid != nil {
				paidStr = paid.Format(time.RFC3339)
			}
			cw.Write([]string{strconv.FormatInt(id, 10), biz, strconv.FormatInt(amount, 10),
				strconv.Itoa(int(status)), paidStr, strconv.FormatInt(refund, 10), strconv.Itoa(int(fstatus)), strconv.Itoa(retry)})
			n++
		}
	case "users":
		cw.Write([]string{"user_id", "identity_type", "status", "realname_status", "admin_role", "region_code", "created_at"})
		rows, err := m.deps.Pool.Query(ctx, `
			SELECT id, identity_type, status, realname_status, admin_role, COALESCE(region_code,''), created_at
			FROM idn_user WHERE created_at >= to_timestamp($1) AND created_at < to_timestamp($2)
			ORDER BY created_at`, from, to)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var it, status, rn, ar int16
			var region string
			var created time.Time
			if err := rows.Scan(&id, &it, &status, &rn, &ar, &region, &created); err != nil {
				return nil, 0, err
			}
			cw.Write([]string{strconv.FormatInt(id, 10), strconv.Itoa(int(it)), strconv.Itoa(int(status)),
				strconv.Itoa(int(rn)), strconv.Itoa(int(ar)), region, created.Format(time.RFC3339)})
			n++
		}
	case "reviews":
		cw.Write([]string{"task_id", "source", "subject_type", "subject_id", "risk_level", "status", "reason", "created_at", "reviewed_at"})
		rows, err := m.deps.Pool.Query(ctx, `
			SELECT id, source, subject_type, subject_id, risk_level, status, COALESCE(reason,''), created_at, reviewed_at
			FROM trs_review_task WHERE created_at >= to_timestamp($1) AND created_at < to_timestamp($2)
			ORDER BY created_at`, from, to)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		for rows.Next() {
			var id, subj int64
			var source, risk, status int16
			var st, reason string
			var created time.Time
			var reviewed *time.Time
			if err := rows.Scan(&id, &source, &st, &subj, &risk, &status, &reason, &created, &reviewed); err != nil {
				return nil, 0, err
			}
			var rv string
			if reviewed != nil {
				rv = reviewed.Format(time.RFC3339)
			}
			cw.Write([]string{strconv.FormatInt(id, 10), strconv.Itoa(int(source)), st,
				strconv.FormatInt(subj, 10), strconv.Itoa(int(risk)), strconv.Itoa(int(status)), reason,
				created.Format(time.RFC3339), rv})
			n++
		}
	case "settlements":
		return nil, 0, fmt.Errorf("导出模板 settlements 待渠道模块开通后启用")
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return nil, 0, err
	}
	// 敏感字段白名单校验：逐字段检查（纯数字列=ID/金额，跳过；文本列禁止手机号/证件号/明文联系方式）
	if bad := checkNoSensitiveCSV(buf.String()); bad != "" {
		return nil, 0, fmt.Errorf("导出内容命中敏感字段白名单校验：%s", bad)
	}
	return buf.Bytes(), n, nil
}

// checkNoSensitiveCSV 逐字段白名单校验：纯数字字段（ID/金额/时间戳）跳过；
// 文本字段禁止手机号/证件号形态（防敏感信息误入导出）。
func checkNoSensitiveCSV(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, cell := range strings.Split(line, ",") {
			cell = strings.TrimSpace(cell)
			if cell == "" || isAllDigits(cell) {
				continue
			}
			if mask.ContainsPhoneLike(cell) || mask.ContainsIDCardLike(cell) {
				return "文本字段命中敏感形态"
			}
		}
	}
	return ""
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
