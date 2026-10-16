package membership

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"catcube/internal/kernel/audit"
	memcontract "catcube/internal/modules/membership/contract"
)

// 代操作（p4 §4.4/§4.5）：转移企业版主账号、强制解除子账号、会员人工调整。
// 全部要求给出原因，并写审计（detail 含 before/after 快照）。双人复核待 RBAC（#13）。

func (m *Module) actor(req *http.Request) int64 { return userIDFrom(req.Context()) }

func (m *Module) recordChange(req *http.Request, action string, targetRef string, before, after map[string]any, reason string, ok bool) {
	if m.deps.Audit == nil {
		return
	}
	res := audit.ResultOK
	if !ok {
		res = audit.ResultFailed
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: m.actor(req), ActorType: audit.ActorAdmin, Channel: "admin", Action: action,
		TargetRef: targetRef, Detail: audit.Change{Before: before, After: after}.Snapshot(reason),
		Result: res, IP: audit.ClientIP(req),
	})
}

// handleTransferMaster 转移企业版主账号（子账号归属一并转移）。
func (m *Module) handleTransferMaster(w http.ResponseWriter, req *http.Request) {
	masterID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		ToUserID string `json:"to_user_id"`
		Reason   string `json:"reason"`
	}
	if masterID == 0 || json.NewDecoder(req.Body).Decode(&body) != nil || body.Reason == "" {
		writeErr(w, codeParamInvalid, "参数错误（必须给原因）")
		return
	}
	to, _ := strconv.ParseInt(body.ToUserID, 10, 64)
	if to == 0 || to == masterID {
		writeErr(w, codeParamInvalid, "目标账号非法")
		return
	}
	before, _, _ := m.svc.Me(req.Context(), masterID)
	targetBefore, targetOK, _ := m.svc.Me(req.Context(), to)
	// 目标须为生效中的企业版（归属只能转给同级账号）
	if !targetOK || targetBefore.Level != memcontract.LevelEnterprise || targetBefore.Status != 1 {
		m.recordChange(req, "membership:master:transfer", userRef(to),
			map[string]any{"active_enterprise": false}, map[string]any{}, body.Reason, false)
		writeErr(w, codeNotEnterprise, "目标账号须为生效中的企业版")
		return
	}
	// 迁移：子账号归属 + 主账号会员记录（原主账号保留会员但降级为非企业主）
	n, err := m.svc.TransferMaster(req.Context(), masterID, to)
	if err != nil {
		m.recordChange(req, "membership:master:transfer", userRef(masterID),
			map[string]any{"master": masterID}, map[string]any{"to": to}, body.Reason, false)
		writeErr(w, codeInternal, "服务错误")
		return
	}
	m.recordChange(req, "membership:master:transfer", userRef(masterID),
		map[string]any{"master": masterID, "level": before.Level}, map[string]any{"master": to, "sub_accounts_moved": n}, body.Reason, true)
	writeData(w, map[string]any{"transferred": true, "sub_accounts_moved": n})
}

// handleForceRemoveSub 强制解除子账号（平台代操作）。
func (m *Module) handleForceRemoveSub(w http.ResponseWriter, req *http.Request) {
	var body struct {
		MasterID  string `json:"master_id"`
		SubUserID string `json:"sub_user_id"`
		Reason    string `json:"reason"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil || body.Reason == "" {
		writeErr(w, codeParamInvalid, "参数错误（必须给原因）")
		return
	}
	master, _ := strconv.ParseInt(body.MasterID, 10, 64)
	sub, _ := strconv.ParseInt(body.SubUserID, 10, 64)
	if master == 0 || sub == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.RemoveSub(req.Context(), master, sub); err != nil {
		m.recordChange(req, "membership:sub:remove", fmt.Sprintf(`{"id":"%d","type":"sub_account"}`, sub),
			map[string]any{"master_id": master, "status": "active"}, map[string]any{}, body.Reason, false)
		writeErr(w, codeInternal, "服务错误或非企业版")
		return
	}
	m.recordChange(req, "membership:sub:remove", fmt.Sprintf(`{"id":"%d","type":"sub_account"}`, sub),
		map[string]any{"master_id": master, "status": "active"}, map[string]any{"status": "removed"}, body.Reason, true)
	writeData(w, map[string]any{"removed": true})
}

// handleAdjustMember 会员人工调整（等级/到期时间；双人复核待 #13）。
func (m *Module) handleAdjustMember(w http.ResponseWriter, req *http.Request) {
	userID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		Level    *int16 `json:"level"`
		ExpireAt string `json:"expire_at"` // RFC3339；为空表示不改
		Reason   string `json:"reason"`
	}
	if userID == 0 || json.NewDecoder(req.Body).Decode(&body) != nil || body.Reason == "" {
		writeErr(w, codeParamInvalid, "参数错误（必须给原因）")
		return
	}
	before, ok, err := m.svc.Me(req.Context(), userID)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	beforeSnap := map[string]any{"member": ok, "level": before.Level, "expires_at": before.ExpiresAt.Format(time.RFC3339)}
	var expire *time.Time
	if body.ExpireAt != "" {
		t, perr := time.Parse(time.RFC3339, body.ExpireAt)
		if perr != nil {
			writeErr(w, codeParamInvalid, "到期时间格式应为 RFC3339")
			return
		}
		expire = &t
	}
	if err := m.svc.AdjustMember(req.Context(), userID, body.Level, expire); err != nil {
		m.recordChange(req, "membership:member:adjust", userRef(userID), beforeSnap, map[string]any{}, body.Reason, false)
		writeErr(w, codeInternal, "服务错误")
		return
	}
	after, _, _ := m.svc.Me(req.Context(), userID)
	afterSnap := map[string]any{"member": true, "level": after.Level, "expires_at": after.ExpiresAt.Format(time.RFC3339)}
	m.recordChange(req, "membership:member:adjust", userRef(userID), beforeSnap, afterSnap, body.Reason, true)
	writeData(w, map[string]any{"adjusted": true})
}

func userRef(id int64) string {
	return fmt.Sprintf(`{"id":"%d","type":"user"}`, id)
}
