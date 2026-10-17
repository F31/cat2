package verify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"catcube/internal/kernel/audit"
	"catcube/internal/modules/verify/contract"
)

// ---- 认证审核（企业/平台）----

func (m *Module) handleOrgReviewCert(w http.ResponseWriter, req *http.Request) {
	m.reviewCert(w, req, contract.ReviewerOrg)
}

func (m *Module) handlePlatformReviewCert(w http.ResponseWriter, req *http.Request) {
	m.reviewCert(w, req, contract.ReviewerPlatform)
}

func (m *Module) reviewCert(w http.ResponseWriter, req *http.Request, reviewerType int16) {
	certID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		Pass   bool   `json:"pass"`
		Reason string `json:"reason"`
	}
	if certID == 0 || json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.ReviewCert(req.Context(), certID, userIDFrom(req.Context()), reviewerType, body.Pass, body.Reason); err != nil {
		logErr("cert review", err)
		m.auditVerify(req, reviewerType, "verify:cert:review", fmt.Sprintf(`{"type":"cert","id":"%d"}`, certID),
			map[string]any{"pass": body.Pass, "reviewer_type": reviewerType, "err": err.Error()}, audit.ResultDenied)
		writeVerifyErrExt(w, err)
		return
	}
	m.auditVerify(req, reviewerType, "verify:cert:review", fmt.Sprintf(`{"type":"cert","id":"%d"}`, certID),
		map[string]any{"pass": body.Pass, "reviewer_type": reviewerType, "reason": body.Reason}, audit.ResultOK)
	writeData(w, map[string]any{"reviewed": true})
}

func (m *Module) handlePendingCerts(w http.ResponseWriter, req *http.Request) {
	rt, _ := strconv.Atoi(req.URL.Query().Get("reviewer_type"))
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	items, err := m.svc.PendingCerts(req.Context(), int16(rt), 0, limit)
	if err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

// ---- 一人多单位（归属核验）----

func (m *Module) handleApplyAffiliation(w http.ResponseWriter, req *http.Request) {
	var body struct {
		OrgID        string  `json:"org_id"`
		Role         string  `json:"role"`
		ProofFileIDs []int64 `json:"proof_file_ids"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	orgID, _ := strconv.ParseInt(body.OrgID, 10, 64)
	aff, err := m.svc.ApplyAffiliation(req.Context(), userIDFrom(req.Context()), orgID, body.Role, body.ProofFileIDs)
	if err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, aff)
}

func (m *Module) handleMyAffiliations(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.AffiliationsOf(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handlePendingAffiliations(w http.ResponseWriter, req *http.Request) {
	orgID, _ := strconv.ParseInt(req.URL.Query().Get("org_id"), 10, 64)
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	items, err := m.svc.PendingAffiliations(req.Context(), orgID, userIDFrom(req.Context()), limit)
	if err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleVerifyAffiliation(w http.ResponseWriter, req *http.Request) {
	affID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		Pass   bool   `json:"pass"`
		Reason string `json:"reason"`
	}
	if affID == 0 || json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.VerifyAffiliation(req.Context(), affID, userIDFrom(req.Context()), body.Pass, body.Reason); err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"verified": true})
}

func (m *Module) handleSetPrimary(w http.ResponseWriter, req *http.Request) {
	affID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err := m.svc.SetPrimaryAffiliation(req.Context(), userIDFrom(req.Context()), affID); err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"primary": true})
}

func (m *Module) handleResolveAffiliation(w http.ResponseWriter, req *http.Request) {
	affID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		Pass   bool   `json:"pass"`
		Reason string `json:"reason"`
	}
	if affID == 0 || json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.ResolveAffiliation(req.Context(), affID, userIDFrom(req.Context()), body.Pass, body.Reason); err != nil {
		m.auditVerify(req, contract.ReviewerPlatform, "verify:affiliation:resolve", fmt.Sprintf(`{"type":"affiliation","id":"%d"}`, affID),
			map[string]any{"pass": body.Pass, "err": err.Error()}, audit.ResultFailed)
		writeVerifyErrExt(w, err)
		return
	}
	m.auditVerify(req, contract.ReviewerPlatform, "verify:affiliation:resolve", fmt.Sprintf(`{"type":"affiliation","id":"%d"}`, affID),
		map[string]any{"pass": body.Pass, "reason": body.Reason}, audit.ResultOK)
	writeData(w, map[string]any{"resolved": true})
}

// ---- 组织管理员机制 ----

func (m *Module) handleInviteOrgAdmin(w http.ResponseWriter, req *http.Request) {
	orgID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		UserID string `json:"user_id"`
	}
	if orgID == 0 || json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	invitee, _ := strconv.ParseInt(body.UserID, 10, 64)
	if err := m.svc.InviteOrgAdmin(req.Context(), orgID, userIDFrom(req.Context()), invitee); err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"invited": true})
}

func (m *Module) handleOrgAdmins(w http.ResponseWriter, req *http.Request) {
	orgID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	items, err := m.svc.OrgAdminsOf(req.Context(), orgID)
	if err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleConfirmOrgAdmin(w http.ResponseWriter, req *http.Request) {
	inviteID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if inviteID == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.ConfirmOrgAdmin(req.Context(), inviteID, userIDFrom(req.Context())); err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"confirmed": true})
}

func (m *Module) handleResetOrgAdmin(w http.ResponseWriter, req *http.Request) {
	orgID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		UserID string `json:"user_id"`
		Remove bool   `json:"remove"`
	}
	if orgID == 0 || json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	uid, _ := strconv.ParseInt(body.UserID, 10, 64)
	if err := m.svc.ResetOrgAdmin(req.Context(), orgID, userIDFrom(req.Context()), uid, body.Remove); err != nil {
		m.auditVerify(req, contract.ReviewerPlatform, "verify:org_admin:reset", fmt.Sprintf(`{"type":"org","id":"%d"}`, orgID),
			map[string]any{"user_id": strconv.FormatInt(uid, 10), "remove": body.Remove, "err": err.Error()}, audit.ResultFailed)
		writeVerifyErrExt(w, err)
		return
	}
	m.auditVerify(req, contract.ReviewerPlatform, "verify:org_admin:reset", fmt.Sprintf(`{"type":"org","id":"%d"}`, orgID),
		map[string]any{"user_id": strconv.FormatInt(uid, 10), "remove": body.Remove}, audit.ResultOK)
	writeData(w, map[string]any{"reset": true})
}

// auditVerify 认证/标识类操作审计（平台=admin，企业审核=mp 用户）。
func (m *Module) auditVerify(req *http.Request, reviewerType int16, action, targetRef string, detail map[string]any, result audit.Result) {
	if m.deps.Audit == nil {
		return
	}
	actorType, channel := audit.ActorAdmin, "admin"
	if reviewerType == contract.ReviewerOrg {
		actorType, channel = audit.ActorUser, "mp"
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: userIDFrom(req.Context()), ActorType: actorType, Channel: channel, Action: action,
		TargetRef: targetRef, Detail: detail, Result: result,
		IP: audit.ClientIP(req), UserAgent: req.UserAgent(), RequestID: audit.RequestIDOf(req.Context()),
	})
}

var _ = fmt.Sprintf

// ---- 认证标识配置（p4 §6.7）----

func (m *Module) handleBadgeDefs(w http.ResponseWriter, req *http.Request) {
	defs, err := m.svc.BadgeDefs(req.Context())
	if err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"items": defs})
}

func (m *Module) handleUpsertBadge(w http.ResponseWriter, req *http.Request) {
	code := req.PathValue("code")
	var body struct {
		Label string          `json:"label"`
		Level int16           `json:"level"`
		Style json.RawMessage `json:"style"`
	}
	if code == "" || json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.UpsertBadgeDef(req.Context(), code, body.Label, body.Level, body.Style); err != nil {
		m.auditVerify(req, contract.ReviewerPlatform, "verify:badge:config", `{"type":"badge","id":"`+code+`"}`,
			map[string]any{"label": body.Label, "level": body.Level, "err": err.Error()}, audit.ResultFailed)
		writeVerifyErrExt(w, err)
		return
	}
	m.auditVerify(req, contract.ReviewerPlatform, "verify:badge:config", `{"type":"badge","id":"`+code+`"}`,
		map[string]any{"label": body.Label, "level": body.Level}, audit.ResultOK)
	writeData(w, map[string]any{"saved": true})
}

func (m *Module) handleGrantBadge(w http.ResponseWriter, req *http.Request) {
	var body struct {
		SubjectType string `json:"subject_type"`
		SubjectID   string `json:"subject_id"`
		Code        string `json:"code"`
		Grant       bool   `json:"grant"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	sid, _ := strconv.ParseInt(body.SubjectID, 10, 64)
	if err := m.svc.SetBadgeGrant(req.Context(), body.SubjectType, sid, body.Code, body.Grant); err != nil {
		m.auditVerify(req, contract.ReviewerPlatform, "verify:badge:grant", fmt.Sprintf(`{"type":"%s","id":"%d"}`, body.SubjectType, sid),
			map[string]any{"code": body.Code, "grant": body.Grant, "err": err.Error()}, audit.ResultFailed)
		writeVerifyErrExt(w, err)
		return
	}
	m.auditVerify(req, contract.ReviewerPlatform, "verify:badge:grant", fmt.Sprintf(`{"type":"%s","id":"%d"}`, body.SubjectType, sid),
		map[string]any{"code": body.Code, "grant": body.Grant}, audit.ResultOK)
	writeData(w, map[string]any{"ok": true})
}

func (m *Module) handleOverdueAffiliations(w http.ResponseWriter, req *http.Request) {
	days, _ := strconv.Atoi(req.URL.Query().Get("days"))
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	items, err := m.svc.OverdueAffiliations(req.Context(), days, limit)
	if err != nil {
		writeVerifyErrExt(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}
