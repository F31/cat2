// Package identity R0 账号模块：微信登录注册一体化、令牌与会话、authz_ver、账号状态、注销。
// 实名核验属 verify（R2）；本模块只暴露 AuthzBump 供其回写。
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/kernel/settings"
	"catcube/internal/modules/identity/contract"
	"catcube/internal/modules/identity/internal/app"
	"catcube/internal/modules/identity/internal/domain"
	"catcube/internal/modules/identity/internal/infra"
	"catcube/internal/ports"
	"catcube/pkg/ids"
	"catcube/pkg/mask"
)

// Deps 装配依赖。
type Deps struct {
	Pool       *db.Pool
	WeChat     ports.WeChatAuth
	PhoneCode  app.PhoneExchanger // 手机号获取（新版 code 接口；开发态为假实现）
	Bus        *eventbus.Bus
	Encryptor  *sensitive.Encryptor // 手机号/证件号字段加密（生产必配）
	BlindKey   []byte               // phone_hmac 独立密钥
	Notify     ports.Notify
	DevSMSCode string
	Audit      *audit.Recorder    // 后台封禁/解封操作审计
	Settings   *settings.Registry // 登录失败阈值等安全配置（p4 §3.4）
}

// Module identity 模块。
type Module struct {
	deps  Deps
	token *app.TokenService
	svc   *app.Service
	store *infra.Store
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "identity" }
func (m *Module) Rank() module.Rank   { return module.RankR0 }
func (m *Module) DependsOn() []string { return []string{"dict"} }

// Register 注册路由与能力提供者。
func (m *Module) Register(r *module.Registry) error {
	ts, err := app.NewTokenService()
	if err != nil {
		return err
	}
	m.token = ts
	m.store = infra.NewStore(m.deps.Pool)
	exchanger := m.deps.PhoneCode
	if exchanger == nil {
		exchanger = realPhoneExchanger{m.deps.WeChat}
	}
	m.svc = app.NewService(app.Options{
		Repo: m.store, Pool: m.deps.Pool, Tokens: ts, WeChat: exchanger,
		Encryptor: m.deps.Encryptor, BlindKey: m.deps.BlindKey, Bus: m.deps.Bus,
		Notify: m.deps.Notify, DevSMSCode: m.deps.DevSMSCode,
		FailThreshold: func(ctx context.Context) int {
			if m.deps.Settings == nil {
				return domain.FailThreshold
			}
			if v, err := m.deps.Settings.Int(ctx, 1, "admin.security.login_fail_threshold"); err == nil && v > 0 {
				return int(v)
			}
			return domain.FailThreshold
		},
	})

	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/auth/wechat", Handler: m.handleWechatLogin},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/auth/phone/code", Handler: m.handleSMSCode},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/auth/phone", Handler: m.handleSMSLogin},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/auth/refresh", Handler: m.handleRefresh},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/users/me", Handler: m.auth(m.handleMe)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/users/me/deactivate", Handler: m.auth(m.handleDeactivate)},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/users", Handler: m.auth(m.handleAdminUsers), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/users/{id}/ban", Handler: m.auth(m.handleAdminBan), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/users/{id}/unban", Handler: m.auth(m.handleAdminUnban), Admin: true},
	)
	// CapabilityProvider：账号状态（实名能力在 verify 注册，此处恒 false）
	r.AuditPoints = append(r.AuditPoints,
		audit.Point{Action: "identity:user:ban", Path: "/api/admin/v1/users/{id}/ban", Required: true, LogFailed: true, Module: "identity"},
		audit.Point{Action: "identity:user:unban", Path: "/api/admin/v1/users/{id}/unban", Required: true, LogFailed: true, Module: "identity"},
		audit.Point{Action: "identity:user:deactivate", Path: "/api/mp/v1/users/me/deactivate", Required: true, LogFailed: false, Module: "identity"},
		audit.Point{Action: "identity:login:locked", Path: "/api/mp/v1/auth/phone", Required: true, LogFailed: true, Module: "identity"},
	)
	r.CapProviders = append(r.CapProviders, capProvider{m})
	// 会员权益变化 → 递增授权版本，消除令牌陈旧窗口（topic 按契约；R0 不 import membership R3）
	r.EventSubs = append(r.EventSubs,
		eventbus.Subscription{Topic: memberActivatedTopic, Handler: m.onMembershipChange},
		eventbus.Subscription{Topic: memberExpiredTopic, Handler: m.onMembershipChange},
	)
	return nil
}

// 会员事件 topic（与 membership 契约一致，避免跨阶 import）。
const (
	memberActivatedTopic = "membership.MemberActivated.v1"
	memberExpiredTopic   = "membership.MemberExpired.v1"
)

// onMembershipChange 会员生效/过期 → AuthzBump。
func (m *Module) onMembershipChange(ctx context.Context, e eventbus.Event) error {
	var p struct {
		UserID int64 `json:"user_id,string"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.UserID == 0 {
		return nil
	}
	return m.AuthzBump(ctx, p.UserID)
}

func (m *Module) Migrations() string { return migrations }

// IsAdmin 是否平台管理员。
func (m *Module) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	var role int16
	err := m.deps.Pool.QueryRow(ctx, `SELECT admin_role FROM idn_user WHERE id=$1`, userID).Scan(&role)
	if err != nil {
		return false, nil
	}
	return role > 0, nil
}

// AdminStepUpRequired 后台账号是否需短信二次验证（p4 §3.4）。
func (m *Module) AdminStepUpRequired(ctx context.Context, userID int64) (bool, error) {
	return m.svc.AdminStepUpRequired(ctx, userID)
}

// SendStepUpCode 发送后台二次验证短信。
func (m *Module) SendStepUpCode(ctx context.Context, userID int64) error {
	return m.svc.SendStepUpCode(ctx, userID)
}

// VerifyStepUpCode 校验后台二次验证码。
func (m *Module) VerifyStepUpCode(ctx context.Context, userID int64, code string) error {
	return m.svc.VerifyStepUpCode(ctx, userID, code)
}

// GrantAdmin 授予/取消平台管理员。
func (m *Module) GrantAdmin(ctx context.Context, userID int64, admin bool) error {
	r := 0
	if admin {
		r = 1
	}
	_, err := m.deps.Pool.Exec(ctx, `UPDATE idn_user SET admin_role=$2, authz_ver=authz_ver+1, updated_at=now() WHERE id=$1`, userID, r)
	return err
}

// AdminSearchUsers 后台用户检索（手机号经盲索引精确匹配）。
func (m *Module) AdminSearchUsers(ctx context.Context, f contract.AdminUserFilter) ([]contract.AdminUserRow, error) {
	var phoneHMAC []byte
	if f.Phone != "" {
		phoneHMAC = sensitive.BlindIndex(m.deps.BlindKey, f.Phone)
	}
	rows, err := m.store.AdminSearchUsers(ctx, infra.AdminUserFilter{
		Status: f.Status, Q: f.Q, PhoneHMAC: phoneHMAC, RealnameState: f.RealnameStatus,
		IdentityType: f.IdentityType, FromUnix: f.FromUnix, ToUnix: f.ToUnix, Limit: f.Limit, Offset: f.Offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]contract.AdminUserRow, 0, len(rows))
	for _, u := range rows {
		var created int64
		if !u.CreatedAt.IsZero() {
			created = u.CreatedAt.Unix()
		}
		out = append(out, contract.AdminUserRow{ID: u.ID, Nickname: u.Nickname, Status: u.Status,
			IdentityType: u.IdentityType, RealnameStatus: u.RealnameStatus, CreatedAt: created})
	}
	return out, nil
}

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

// Authenticate 校验 Access Token（签名/有效期）。
// UserIDByPhone 按盲索引查已有用户（不建号）。
func (m *Module) UserIDByPhone(ctx context.Context, phone string) (int64, error) {
	u, err := m.store.FindUserByPhoneHMAC(ctx, sensitive.BlindIndex(m.deps.BlindKey, phone))
	if err != nil {
		return 0, err
	}
	return u.ID, nil
}

// IssueTokens 为已认证用户签发会话。
func (m *Module) IssueTokens(ctx context.Context, userID int64, device string) (contract.Tokens, error) {
	return m.svc.IssueTokens(ctx, userID, device)
}

func (m *Module) Authenticate(_ context.Context, accessToken string) (int64, int, error) {
	claims, err := m.token.Verify(accessToken)
	if err != nil {
		return 0, 0, err
	}
	return claims.Sub, claims.AuthzVer, nil
}

// SessionOf 活跃会话数。
func (m *Module) SessionOf(ctx context.Context, userID int64) (int, error) {
	sessions, err := m.store.ActiveSessions(ctx, userID)
	return len(sessions), err
}

// UserBrief 用户摘要。
func (m *Module) UserBrief(ctx context.Context, userID int64) (contract.UserBrief, error) {
	u, err := m.store.UserByID(ctx, userID)
	if err != nil {
		return contract.UserBrief{}, err
	}
	return contract.UserBrief{
		ID: u.ID, Nickname: u.Nickname, Status: int16(u.Status),
		IdentityType: u.IdentityType, AuthzVer: u.AuthzVer,
	}, nil
}

// AuthzBump 权限变更递增授权版本（消除令牌陈旧窗口）。
func (m *Module) AuthzBump(ctx context.Context, userID int64) error {
	_, err := m.deps.Pool.Exec(ctx,
		`UPDATE idn_user SET authz_ver=authz_ver+1, updated_at=now() WHERE id=$1`, userID)
	return err
}

// MarkRealname 回写实名核验结果（verify 经此，不读 idn_user）。
func (m *Module) MarkRealname(ctx context.Context, userID int64, status int16) error {
	_, err := m.deps.Pool.Exec(ctx,
		`UPDATE idn_user SET realname_status=$2, updated_at=now() WHERE id=$1`, userID, status)
	return err
}

// Deactivate 注销：匿名化本模块数据并发布事件（其他模块订阅后各自处理）。
func (m *Module) Deactivate(ctx context.Context, userID int64) error {
	return m.deps.Pool.InTx(ctx, func(tx db.Tx) error {
		if err := m.store.LogDeactivation(ctx, tx, userID, "用户主动注销"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE idn_user SET status=3, nickname='已注销用户', avatar_file_id=NULL,
				wechat_openid=NULL, phone_enc=NULL, phone_hmac=NULL,
				id_card_name_enc=NULL, id_card_no_enc=NULL, updated_at=now()
			WHERE id=$1`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE idn_session SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
			return err
		}
		if m.deps.Bus != nil {
			return m.deps.Bus.Publish(ctx, tx,
				contract.NewEvent(ids.New().String(), contract.EventUserDeactivated, userID))
		}
		return nil
	})
}

// realPhoneExchanger 适配旧版端口（session_key 解密路径）到 PhoneExchanger 接口。
// 新版 getuserphonenumber（code 接口）由 wechat.AuthClient 直接实现。
type realPhoneExchanger struct{ ports.WeChatAuth }

func (r realPhoneExchanger) GetPhoneNumberByCode(ctx context.Context, code string) (string, error) {
	return r.GetPhoneNumber(ctx, "", code, "")
}

// ---- HTTP 层 ----

// identity 错误码（contracts/errors/README.md；数值即 MMNNNN，前导零省略）。
const (
	codeParamInvalid   = 1     // 000001
	codeUnauthorized   = 2     // 000002
	codeForbidden      = 3     // 000003
	codeUpstreamFail   = 8     // 000008
	codeInternal       = 9     // 000009
	codeWeChatCode     = 10001 // 010001 微信 code 无效或已使用
	codeRefreshInvalid = 10002 // 010002 refresh token 失效或被撤销
	codeDeactivated    = 10003 // 010003 账号已注销
	codeBanned         = 10004 // 010004 账号被停用
	codeSMSCodeInvalid = 10005 // 010005 短信验证码无效或已过期
	codeLocked         = 10006 // 010006 登录尝试过于频繁
	codePhoneInvalid   = 10007 // 010007 手机号缺失或非法
)

type loginReq struct {
	Code          string `json:"code"`
	EncryptedData string `json:"encrypted_data"`
	IV            string `json:"iv"`
	PhoneCode     string `json:"phone_code"`
	Device        string `json:"device"`
	IdentityType  int16  `json:"identity_type"` // 注册时生效：1个人/2组织；登录老用户忽略
}

type tokensResp struct {
	UserID int64           `json:"user_id,string"`
	New    bool            `json:"new"`
	Tokens contract.Tokens `json:"tokens"`
}

func (m *Module) handleWechatLogin(w http.ResponseWriter, req *http.Request) {
	var body loginReq
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Code == "" {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	phoneCode := body.PhoneCode
	if phoneCode == "" {
		phoneCode = body.EncryptedData // 兼容旧字段名
	}
	if body.IdentityType < 0 || body.IdentityType > 2 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	res, err := m.svc.WeChatLogin(req.Context(), body.Code, phoneCode, body.Device, clientIP(req), body.IdentityType)
	if err != nil {
		m.auditLoginDenied(req, err)
		writeLoginErr(w, err)
		return
	}
	writeData(w, tokensResp{UserID: res.UserID, New: res.IsNew, Tokens: res.Tokens})
}

type smsReq struct {
	Phone  string `json:"phone"`
	Code   string `json:"code"`
	Device string `json:"device"`
}

func (m *Module) handleSMSCode(w http.ResponseWriter, req *http.Request) {
	var body smsReq
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.svc.RequestSMSCode(req.Context(), body.Phone); err != nil {
		writeLoginErr(w, err)
		return
	}
	writeData(w, map[string]any{"sent": true})
}

func (m *Module) handleSMSLogin(w http.ResponseWriter, req *http.Request) {
	var body smsReq
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	res, err := m.svc.SMSLogin(req.Context(), body.Phone, body.Code, body.Device, clientIP(req))
	if err != nil {
		m.auditLoginDenied(req, err)
		writeLoginErr(w, err)
		return
	}
	writeData(w, tokensResp{UserID: res.UserID, New: res.IsNew, Tokens: res.Tokens})
}

func (m *Module) handleRefresh(w http.ResponseWriter, req *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	tokens, err := m.svc.Refresh(req.Context(), body.RefreshToken)
	if err != nil {
		writeLoginErr(w, err)
		return
	}
	writeData(w, tokens)
}

func (m *Module) handleMe(w http.ResponseWriter, req *http.Request) {
	uid := userIDFrom(req.Context())
	brief, err := m.UserBrief(req.Context(), uid)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, brief)
}

func (m *Module) handleDeactivate(w http.ResponseWriter, req *http.Request) {
	uid := userIDFrom(req.Context())
	if err := m.Deactivate(req.Context(), uid); err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	if m.deps.Audit != nil {
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: uid, ActorType: audit.ActorUser, Channel: "mp", Action: "identity:user:deactivate",
			TargetRef: fmt.Sprintf(`{"type":"user","id":"%d"}`, uid),
			Result:    audit.ResultOK, IP: clientIP(req), UserAgent: req.UserAgent(),
			RequestID: audit.RequestIDOf(req.Context()),
		})
	}
	writeData(w, map[string]any{"deactivated": true})
}

// auditLoginDenied 登录拒绝（锁定/失败）审计：防爆破与撞库溯源。
func (m *Module) auditLoginDenied(req *http.Request, cause error) {
	if m.deps.Audit == nil || !errors.Is(cause, domain.ErrLocked) {
		return
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: 0, ActorType: audit.ActorUser, Channel: "mp", Action: "identity:login:locked",
		Result: audit.ResultDenied, IP: clientIP(req), UserAgent: req.UserAgent(),
		RequestID: audit.RequestIDOf(req.Context()),
	})
}

// auth 鉴权中间件：Bearer → 验签 → 比对 authz_ver（消除陈旧窗口）→ 账号状态。
func (m *Module) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			writeErr(w, codeUnauthorized, "未登录")
			return
		}
		uid, ver, err := m.Authenticate(req.Context(), token)
		if err != nil {
			writeErr(w, codeUnauthorized, "登录已失效")
			return
		}
		u, err := m.store.UserByID(req.Context(), uid)
		if err != nil {
			writeErr(w, codeUnauthorized, "登录已失效")
			return
		}
		if u.AuthzVer != ver {
			writeErr(w, codeUnauthorized, "权限已变更，请重新登录")
			return
		}
		if !domain.CanLogin(u.Status) {
			if u.Status == domain.StatusDeactivated {
				writeErr(w, codeDeactivated, "账号已注销")
			} else {
				writeErr(w, codeBanned, "账号被停用")
			}
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
	if t := strings.TrimSpace(req.Header.Get("X-Admin-Token")); t != "" {
		return t
	}
	h := req.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func clientIP(req *http.Request) string {
	if v := req.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	host := req.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

// writeLoginErr 错误 → 错误码（见 contracts/errors/README.md 01 段）。
func writeLoginErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrAccountDeactivated):
		writeErr(w, codeDeactivated, "账号已注销")
	case errors.Is(err, domain.ErrAccountBanned):
		writeErr(w, codeBanned, "账号被停用")
	case errors.Is(err, domain.ErrLocked):
		writeErr(w, codeLocked, "尝试过于频繁，请稍后再试")
	case errors.Is(err, domain.ErrCodeInvalid):
		writeErr(w, codeSMSCodeInvalid, "验证码无效或已过期")
	case errors.Is(err, domain.ErrPhoneRequired):
		writeErr(w, codePhoneInvalid, "缺少或非法的手机号")
	case errors.Is(err, domain.ErrRefreshInvalid):
		writeErr(w, codeRefreshInvalid, "refresh token 失效或被撤销")
	case errors.Is(err, domain.ErrWeChatUpstream):
		writeErr(w, codeUpstreamFail, "微信授权失败")
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

// httpStatus 错误码 → HTTP 状态码（contracts/errors/README.md 通用段 + identity 段）。
func httpStatus(code int) int {
	switch code {
	case codeUnauthorized, codeRefreshInvalid:
		return http.StatusUnauthorized
	case codeForbidden, codeDeactivated, codeBanned:
		return http.StatusForbidden
	case codeParamInvalid, codePhoneInvalid, codeSMSCodeInvalid, codeWeChatCode:
		return http.StatusBadRequest
	case codeLocked:
		return http.StatusTooManyRequests
	case codeUpstreamFail:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

func writeErr(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus(code))
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}

// ---- 后台管理（RBAC 细化在 p4；此阶段经登录态鉴权 + Admin 路由标记）----

func (m *Module) maskPhone(enc []byte) string {
	if m.deps.Encryptor == nil || len(enc) == 0 {
		return ""
	}
	plain, err := m.deps.Encryptor.Decrypt(enc)
	if err != nil {
		return ""
	}
	return mask.Phone(string(plain))
}

func (m *Module) handleAdminUsers(w http.ResponseWriter, req *http.Request) {
	status := int16(-1)
	if s := req.URL.Query().Get("status"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			status = int16(v)
		}
	}
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := m.store.AdminListUsers(req.Context(), status, req.URL.Query().Get("q"), limit)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, u := range rows {
		items = append(items, map[string]any{
			"id": strconv.FormatInt(u.ID, 10), "nickname": u.Nickname, "status": u.Status,
			"identity_type": u.IdentityType, "realname_status": u.RealnameStatus,
			"phone_masked": m.maskPhone(u.PhoneEnc), "created_at": u.CreatedAt,
		})
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleAdminBan(w http.ResponseWriter, req *http.Request) {
	m.setUserStatus(w, req, domain.StatusBanned)
}

func (m *Module) handleAdminUnban(w http.ResponseWriter, req *http.Request) {
	m.setUserStatus(w, req, domain.StatusActive)
}

func (m *Module) setUserStatus(w http.ResponseWriter, req *http.Request, status domain.UserStatus) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	if err := m.store.SetStatus(req.Context(), id, int16(status)); err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	// 账号状态变化 → 递增授权版本，旧令牌立即失效
	_ = m.AuthzBump(req.Context(), id)
	m.auditStatus(req, id, status, body.Reason)
	writeData(w, map[string]any{"status": int16(status)})
}

// auditStatus 后台封禁/解封落审计（哈希链）。
func (m *Module) auditStatus(req *http.Request, targetID int64, status domain.UserStatus, reason string) {
	if m.deps.Audit == nil {
		return
	}
	action := "identity:user:ban"
	if status == domain.StatusActive {
		action = "identity:user:unban"
	}
	ip := req.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin",
		Action: action, TargetRef: fmt.Sprintf(`{"type":"user","id":"%d"}`, targetID),
		Detail: map[string]any{"reason": reason}, Result: audit.ResultOK,
		IP: ip, UserAgent: req.UserAgent(),
	})
}

// capProvider 账号状态能力提供者。
type capProvider struct{ m *Module }

func (p capProvider) Name() string { return "identity" }

func (p capProvider) Has(ctx context.Context, userID int64, capability string) (bool, error) {
	switch capability {
	case "account.active":
		b, err := p.m.UserBrief(ctx, userID)
		if err != nil {
			return false, err
		}
		return b.Status == 0, nil
	case "realname": // 实名能力在 verify（R2）注册；此处恒 false
		return false, nil
	default:
		return false, nil
	}
}

var _ contract.Facade = (*Module)(nil)
