package app

import (
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
	idcontract "catcube/internal/modules/identity/contract"
)

const adminPasswordIter = 210000

// HashAdminPassword 生成 PBKDF2-SHA256 口令哈希（格式：pbkdf2-sha256$iter$saltHex$hashHex）。
func HashAdminPassword(password string) (string, error) {
	if len(password) < 10 {
		return "", fmt.Errorf("后台密码至少 10 位")
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

// adminPasswordLogin 后台口令登录：手机号 + 密码（PBKDF2 哈希经 env 注入，单管理员起步）。
func adminPasswordLogin(id idcontract.Facade, cfg config.Config, rec *audit.Recorder, lim *loginLimiter) http.HandlerFunc {
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
		if cfg.AdminPhone == "" || cfg.AdminPasswordHash == "" {
			write(w, http.StatusServiceUnavailable, 9, "后台口令登录未配置", nil)
			return
		}
		var body reqBody
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			write(w, http.StatusBadRequest, 1, "参数错误", nil)
			return
		}
		phone := strings.TrimSpace(body.Phone)
		ok := phone == strings.TrimSpace(cfg.AdminPhone) && VerifyAdminPassword(cfg.AdminPasswordHash, body.Password)
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
