package app

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/config"
	"catcube/internal/kernel/obs"
	"catcube/internal/kernel/settings"
	idcontract "catcube/internal/modules/identity/contract"
)

const (
	adminPasswordIter   = 210000
	adminPasswordMinLen = 8
)

// HashAdminPassword 生成 PBKDF2-SHA256 口令哈希（格式：pbkdf2-sha256$iter$saltHex$hashHex）。
func HashAdminPassword(password string) (string, error) {
	if len(password) < adminPasswordMinLen {
		return "", fmt.Errorf("后台密码至少 %d 位", adminPasswordMinLen)
	}
	salt := make([]byte, 16)
	if _, err := randRead(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, adminPasswordIter, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", adminPasswordIter, hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

// VerifyAdminPassword 校验口令（常量时间比较）。
func VerifyAdminPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 10000 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// loginLimiter 后台口令登录失败限频（按 IP，5 次/10 分钟）。
type loginLimiter struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{fails: map[string][]time.Time{}} }

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := time.Now().Add(-10 * time.Minute)
	kept := l.fails[ip][:0]
	for _, t := range l.fails[ip] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	l.fails[ip] = kept
	return len(kept) < 5
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	l.fails[ip] = append(l.fails[ip], time.Now())
	l.mu.Unlock()
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	delete(l.fails, ip)
	l.mu.Unlock()
}

type smsSendLimiter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newSMSSendLimiter() *smsSendLimiter { return &smsSendLimiter{last: map[string]time.Time{}} }

func (l *smsSendLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if t, ok := l.last[key]; ok && now.Sub(t) < time.Minute {
		return false
	}
	l.last[key] = now
	return true
}

// currentAdminPasswordHash 优先使用后台修改后的 DB 哈希，未修改时回退 env。
func currentAdminPasswordHash(ctx context.Context, cfg config.Config, reg *settings.Registry) string {
	hash := strings.TrimSpace(cfg.AdminPasswordHash)
	if reg != nil {
		if v, err := reg.Get(ctx, 1, settings.AdminPasswordHashKey); err == nil && strings.TrimSpace(v) != "" {
			hash = strings.TrimSpace(v)
		}
	}
	return hash
}

// adminPasswordLogin 后台口令登录：手机号 + 密码（PBKDF2 哈希经 env 注入，单管理员起步）。
func adminPasswordLogin(id idcontract.Facade, cfg config.Config, reg *settings.Registry, rec *audit.Recorder, lim *loginLimiter) http.HandlerFunc {
	type reqBody struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	type respTokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	write := func(w http.ResponseWriter, status int, code int, msg string, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": msg, "data": data})
	}
	return func(w http.ResponseWriter, req *http.Request) {
		ip := clientIP(req)
		if !lim.allow(ip) {
			http.Error(w, `{"code":3,"message":"尝试过于频繁，请稍后再试"}`, http.StatusTooManyRequests)
			return
		}
		adminHash := currentAdminPasswordHash(req.Context(), cfg, reg)
		if cfg.AdminPhone == "" || adminHash == "" {
			write(w, http.StatusServiceUnavailable, 9, "后台口令登录未配置", nil)
			return
		}
		var body reqBody
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			write(w, http.StatusBadRequest, 1, "参数错误", nil)
			return
		}
		phone := strings.TrimSpace(body.Phone)
		ok := phone == strings.TrimSpace(cfg.AdminPhone) && VerifyAdminPassword(adminHash, body.Password)
		if !ok {
			lim.fail(ip)
			recordAdminDenied(req, rec, 0, "admin:login:password")
			write(w, http.StatusUnauthorized, 2, "手机号或密码错误", nil)
			return
		}
		uid, err := id.UserIDByPhone(req.Context(), phone)
		if err != nil {
			lim.fail(ip)
			recordAdminDenied(req, rec, 0, "admin:login:password")
			write(w, http.StatusUnauthorized, 2, "手机号或密码错误", nil)
			return
		}
		tokens, err := id.IssueTokens(req.Context(), uid, "admin-web")
		if err != nil {
			write(w, http.StatusForbidden, 4, "账号不可用", nil)
			return
		}
		lim.reset(ip)
		if rec != nil {
			_ = rec.Record(req.Context(), nil, audit.Entry{
				ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: "admin:login:password",
				Result: audit.ResultOK, IP: ip, RequestID: obs.RequestID(req.Context()),
			})
		}
		write(w, http.StatusOK, 0, "ok", map[string]any{
			"user_id": strconv.FormatInt(uid, 10),
			"tokens":  respTokens{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, ExpiresIn: tokens.ExpiresIn},
		})
	}
}

// adminPasswordChange 修改后台主管理员密码：校验旧密码 → 写入 DB 哈希（热生效）。
func adminPasswordChange(id idcontract.Facade, cfg config.Config, reg *settings.Registry, rec *audit.Recorder, lim *loginLimiter) http.HandlerFunc {
	type reqBody struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	write := func(w http.ResponseWriter, status int, code int, msg string, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": msg, "data": data})
	}
	recordFailed := func(req *http.Request, uid int64) {
		if rec == nil {
			return
		}
		_ = rec.Record(req.Context(), nil, audit.Entry{
			ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: "admin:password:change",
			Result: audit.ResultFailed, IP: clientIP(req), RequestID: obs.RequestID(req.Context()),
		})
	}
	return func(w http.ResponseWriter, req *http.Request) {
		uid, ok := adminUID(req)
		if !ok {
			write(w, http.StatusUnauthorized, 2, "未登录", nil)
			return
		}
		if lim != nil && !lim.allow(clientIP(req)) {
			write(w, http.StatusTooManyRequests, 3, "尝试过于频繁，请稍后再试", nil)
			return
		}
		if cfg.AdminPhone == "" {
			write(w, http.StatusServiceUnavailable, 9, "后台口令登录未配置", nil)
			return
		}
		adminID, err := id.UserIDByPhone(req.Context(), strings.TrimSpace(cfg.AdminPhone))
		if err != nil || adminID != uid {
			recordAdminDenied(req, rec, uid, "admin:password:change")
			write(w, http.StatusForbidden, 3, "仅主管理员可修改后台密码", nil)
			return
		}
		var body reqBody
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			write(w, http.StatusBadRequest, 1, "参数错误", nil)
			return
		}
		if len(body.NewPassword) < adminPasswordMinLen {
			write(w, http.StatusBadRequest, 1, fmt.Sprintf("新密码至少 %d 位", adminPasswordMinLen), nil)
			return
		}
		if len(body.NewPassword) > 128 {
			write(w, http.StatusBadRequest, 1, "新密码过长", nil)
			return
		}
		if body.NewPassword == body.OldPassword {
			write(w, http.StatusBadRequest, 1, "新密码不能与当前密码相同", nil)
			return
		}
		current := currentAdminPasswordHash(req.Context(), cfg, reg)
		if current == "" {
			write(w, http.StatusServiceUnavailable, 9, "后台口令登录未配置", nil)
			return
		}
		if !VerifyAdminPassword(current, body.OldPassword) {
			if lim != nil {
				lim.fail(clientIP(req))
			}
			recordAdminDenied(req, rec, uid, "admin:password:change")
			write(w, http.StatusUnauthorized, 2, "当前密码错误", nil)
			return
		}
		if reg == nil {
			write(w, http.StatusServiceUnavailable, 9, "配置存储不可用", nil)
			return
		}
		hash, err := HashAdminPassword(body.NewPassword)
		if err != nil {
			write(w, http.StatusBadRequest, 1, err.Error(), nil)
			return
		}
		if err := reg.Set(req.Context(), 1, settings.AdminPasswordHashKey, hash, uid); err != nil {
			recordFailed(req, uid)
			write(w, http.StatusInternalServerError, 9, "密码保存失败", nil)
			return
		}
		if lim != nil {
			lim.reset(clientIP(req))
		}
		if rec != nil {
			_ = rec.Record(req.Context(), nil, audit.Entry{
				ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: "admin:password:change",
				Result: audit.ResultOK, IP: clientIP(req), RequestID: obs.RequestID(req.Context()),
			})
		}
		write(w, http.StatusOK, 0, "ok", map[string]any{"changed": true})
	}
}

// adminPasswordResetCode 发送找回密码短信验证码（仅允许配置的主管理员手机号）。
func adminPasswordResetCode(id idcontract.Facade, cfg config.Config, rec *audit.Recorder, lim *smsSendLimiter) http.HandlerFunc {
	type reqBody struct {
		Phone string `json:"phone"`
	}
	write := func(w http.ResponseWriter, status int, code int, msg string, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": msg, "data": data})
	}
	return func(w http.ResponseWriter, req *http.Request) {
		if cfg.AdminPhone == "" {
			write(w, http.StatusServiceUnavailable, 9, "后台口令登录未配置", nil)
			return
		}
		var body reqBody
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			write(w, http.StatusBadRequest, 1, "参数错误", nil)
			return
		}
		phone := strings.TrimSpace(body.Phone)
		if phone != strings.TrimSpace(cfg.AdminPhone) {
			recordAdminDenied(req, rec, 0, "admin:password:reset:code")
			write(w, http.StatusBadRequest, 1, "账号不存在或不可重置", nil)
			return
		}
		uid, err := id.UserIDByPhone(req.Context(), phone)
		if err != nil {
			recordAdminDenied(req, rec, 0, "admin:password:reset:code")
			write(w, http.StatusBadRequest, 1, "账号不存在或不可重置", nil)
			return
		}
		if lim != nil && !lim.allow(phone+"|"+clientIP(req)) {
			write(w, http.StatusTooManyRequests, 3, "验证码发送过于频繁，请稍后再试", nil)
			return
		}
		if err := id.SendPasswordResetCode(req.Context(), uid); err != nil {
			if rec != nil {
				_ = rec.Record(req.Context(), nil, audit.Entry{
					ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: "admin:password:reset:code",
					Result: audit.ResultFailed, IP: clientIP(req), RequestID: obs.RequestID(req.Context()),
				})
			}
			write(w, http.StatusInternalServerError, 9, "验证码发送失败", nil)
			return
		}
		if rec != nil {
			_ = rec.Record(req.Context(), nil, audit.Entry{
				ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: "admin:password:reset:code",
				Result: audit.ResultOK, IP: clientIP(req), RequestID: obs.RequestID(req.Context()),
			})
		}
		write(w, http.StatusOK, 0, "ok", map[string]any{"sent": true})
	}
}

// adminPasswordReset 通过短信验证码重置主管理员密码（无需登录）。
func adminPasswordReset(id idcontract.Facade, cfg config.Config, reg *settings.Registry, rec *audit.Recorder, lim *loginLimiter) http.HandlerFunc {
	type reqBody struct {
		Phone       string `json:"phone"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
	}
	write := func(w http.ResponseWriter, status int, code int, msg string, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": msg, "data": data})
	}
	return func(w http.ResponseWriter, req *http.Request) {
		if cfg.AdminPhone == "" {
			write(w, http.StatusServiceUnavailable, 9, "后台口令登录未配置", nil)
			return
		}
		ip := clientIP(req)
		if lim != nil && !lim.allow(ip) {
			write(w, http.StatusTooManyRequests, 3, "尝试过于频繁，请稍后再试", nil)
			return
		}
		var body reqBody
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			write(w, http.StatusBadRequest, 1, "参数错误", nil)
			return
		}
		phone := strings.TrimSpace(body.Phone)
		if phone != strings.TrimSpace(cfg.AdminPhone) {
			if lim != nil {
				lim.fail(ip)
			}
			recordAdminDenied(req, rec, 0, "admin:password:reset")
			write(w, http.StatusBadRequest, 1, "账号不存在或不可重置", nil)
			return
		}
		uid, err := id.UserIDByPhone(req.Context(), phone)
		if err != nil {
			if lim != nil {
				lim.fail(ip)
			}
			recordAdminDenied(req, rec, 0, "admin:password:reset")
			write(w, http.StatusBadRequest, 1, "账号不存在或不可重置", nil)
			return
		}
		if len(body.NewPassword) < adminPasswordMinLen {
			write(w, http.StatusBadRequest, 1, fmt.Sprintf("新密码至少 %d 位", adminPasswordMinLen), nil)
			return
		}
		if len(body.NewPassword) > 128 {
			write(w, http.StatusBadRequest, 1, "新密码过长", nil)
			return
		}
		if err := id.VerifyPasswordResetCode(req.Context(), uid, strings.TrimSpace(body.Code)); err != nil {
			if lim != nil {
				lim.fail(ip)
			}
			recordAdminDenied(req, rec, uid, "admin:password:reset")
			write(w, http.StatusBadRequest, 2, "验证码无效或已过期", nil)
			return
		}
		if reg == nil {
			write(w, http.StatusServiceUnavailable, 9, "配置存储不可用", nil)
			return
		}
		hash, err := HashAdminPassword(body.NewPassword)
		if err != nil {
			write(w, http.StatusBadRequest, 1, err.Error(), nil)
			return
		}
		if err := reg.Set(req.Context(), 1, settings.AdminPasswordHashKey, hash, uid); err != nil {
			if rec != nil {
				_ = rec.Record(req.Context(), nil, audit.Entry{
					ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: "admin:password:reset",
					Result: audit.ResultFailed, IP: ip, RequestID: obs.RequestID(req.Context()),
				})
			}
			write(w, http.StatusInternalServerError, 9, "密码保存失败", nil)
			return
		}
		if lim != nil {
			lim.reset(ip)
		}
		if rec != nil {
			_ = rec.Record(req.Context(), nil, audit.Entry{
				ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: "admin:password:reset",
				Result: audit.ResultOK, IP: ip, RequestID: obs.RequestID(req.Context()),
			})
		}
		write(w, http.StatusOK, 0, "ok", map[string]any{"changed": true})
	}
}
