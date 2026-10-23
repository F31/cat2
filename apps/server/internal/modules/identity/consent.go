package identity

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"catcube/internal/kernel/audit"
)

// ConsentVersion 当前协议版本；协议内容或隐私政策变更时必须递增。
const ConsentVersion = "2026-10-24"

var consentVersionRe = regexp.MustCompile(`^[0-9A-Za-z._-]{1,32}$`)

// handleConsent 记录用户对用户协议/隐私政策的同意（按版本幂等留痕）。
func (m *Module) handleConsent(w http.ResponseWriter, req *http.Request) {
	var body struct {
		TermsVersion   string `json:"terms_version"`
		PrivacyVersion string `json:"privacy_version"`
		Agreed         bool   `json:"agreed"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || !body.Agreed ||
		!consentVersionRe.MatchString(body.TermsVersion) || !consentVersionRe.MatchString(body.PrivacyVersion) {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if body.TermsVersion != ConsentVersion || body.PrivacyVersion != ConsentVersion {
		writeErr(w, codeParamInvalid, "协议版本已更新，请重新阅读")
		return
	}
	uid := userIDFrom(req.Context())
	for _, doc := range []string{"terms", "privacy"} {
		if _, err := m.deps.Pool.Exec(req.Context(), `
			INSERT INTO idn_consent (user_id, doc_type, version, ip, user_agent)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (user_id, doc_type, version) DO NOTHING`,
			uid, doc, ConsentVersion, clientIP(req), strings.TrimSpace(req.UserAgent())); err != nil {
			writeErr(w, codeInternal, "服务错误")
			return
		}
	}
	if m.deps.Audit != nil {
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: uid, ActorType: audit.ActorUser, Channel: "mp",
			Action: "identity:consent:record", Result: audit.ResultOK,
			IP: clientIP(req), UserAgent: req.UserAgent(), RequestID: audit.RequestIDOf(req.Context()),
			Detail: map[string]any{"version": ConsentVersion, "docs": []string{"terms", "privacy"}},
		})
	}
	writeData(w, map[string]any{"version": ConsentVersion, "recorded": true})
}
