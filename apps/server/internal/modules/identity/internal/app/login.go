// Package app identity 应用服务：登录注册一体化、令牌轮换、短信备选、注销。
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/obs"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/modules/identity/contract"
	"catcube/internal/modules/identity/internal/domain"
	"catcube/internal/modules/identity/internal/infra"
	"catcube/internal/ports"
	"catcube/pkg/ids"
)

// RefreshTTL refresh token 有效期（滚动轮换）。
const RefreshTTL = 7 * 24 * time.Hour

// SMSCodeTTL 验证码有效期。
const SMSCodeTTL = 5 * time.Minute

// PhoneExchanger 微信手机号获取（真实实现与开发态假实现都满足）。
type PhoneExchanger interface {
	Code2Session(ctx context.Context, code string) (openID, sessionKey string, err error)
	GetPhoneNumberByCode(ctx context.Context, code string) (string, error)
}

// Repo 应用层所需仓储能力（PG 实现见 infra.Store）。
type Repo interface {
	FindUserByPhoneHMAC(ctx context.Context, hmac []byte) (infra.User, error)
	UserByID(ctx context.Context, id int64) (infra.User, error)
	CreateUser(ctx context.Context, tx db.Tx, u infra.User) (int64, error)
	BindOpenID(ctx context.Context, tx db.Tx, userID int64, openID string) error
	TouchActive(ctx context.Context, userID int64) error
	ActiveSessions(ctx context.Context, userID int64) ([]domain.SessionBrief, error)
	RevokeSessions(ctx context.Context, tx db.Tx, ids []int64) error
	CreateSession(ctx context.Context, tx db.Tx, userID int64, refreshHash []byte, device string, ttl time.Duration) (int64, error)
	SessionByRefreshHash(ctx context.Context, hash []byte) (infra.Session, error)
	RotateSession(ctx context.Context, tx db.Tx, sessionID int64, newHash []byte, ttl time.Duration) error
	RecordAttempt(ctx context.Context, tx db.Tx, userID int64, phoneHMAC []byte, success bool, ip string) error
	RecentFailures(ctx context.Context, phoneHMAC []byte) (int, time.Time, error)
	FailuresSinceLastSuccess(ctx context.Context, phoneHMAC []byte) (int, error)
	SaveSMSCode(ctx context.Context, phoneHMAC, codeHash []byte, ttl time.Duration) error
	ConsumeSMSCode(ctx context.Context, phoneHMAC, codeHash []byte) (bool, error)
}

// Service 身份应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	tokens   *TokenService
	wechat   PhoneExchanger
	enc      *sensitive.Encryptor
	blindKey []byte
	bus      *eventbus.Bus
	notify   ports.Notify
	// DevSMSCode 非空时作为固定验证码（开发/联调），生产留空走真实短信。
	DevSMSCode string
	// failThreshold 登录失败阈值（超管可配；p4 §3.4）。
	failThreshold func(ctx context.Context) int
}

// Options 构造参数。
type Options struct {
	Repo       Repo
	Pool       *db.Pool
	Tokens     *TokenService
	WeChat     PhoneExchanger
	Encryptor  *sensitive.Encryptor
	BlindKey   []byte
	Bus        *eventbus.Bus
	Notify     ports.Notify
	DevSMSCode string
	// FailThreshold 登录失败阈值提供者（缺省 5）。
	FailThreshold func(ctx context.Context) int
}

func NewService(o Options) *Service {
	return &Service{
		repo: o.Repo, pool: o.Pool, tokens: o.Tokens, wechat: o.WeChat,
		enc: o.Encryptor, blindKey: o.BlindKey, bus: o.Bus, notify: o.Notify,
		DevSMSCode: o.DevSMSCode, failThreshold: o.FailThreshold,
	}
}

// threshold 当前登录失败阈值（缺省 domain.FailThreshold）。
func (s *Service) threshold(ctx context.Context) int {
	if s.failThreshold != nil {
		if v := s.failThreshold(ctx); v > 0 {
			return v
		}
	}
	return domain.FailThreshold
}

// AdminStepUpRequired 后台账号是否需短信二次验证（自上次成功以来失败达阈值；p4 §3.4）。
func (s *Service) AdminStepUpRequired(ctx context.Context, userID int64) (bool, error) {
	u, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		return false, nil // 无此用户按无需处理
	}
	n, err := s.repo.FailuresSinceLastSuccess(ctx, u.PhoneHMAC)
	if err != nil {
		return false, err
	}
	return n >= s.threshold(ctx), nil
}

// SendStepUpCode 向账号绑定手机号发送二次验证码（复用登录短信码通道）。
func (s *Service) SendStepUpCode(ctx context.Context, userID int64) error {
	u, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		return domain.ErrPhoneRequired
	}
	phone, err := s.plainPhone(u)
	if err != nil || phone == "" {
		return domain.ErrPhoneRequired
	}
	return s.RequestSMSCode(ctx, phone)
}

// VerifyStepUpCode 校验二次验证码；成功记录一次成功尝试（清除失败计数）。
func (s *Service) VerifyStepUpCode(ctx context.Context, userID int64, code string) error {
	if code == "" {
		return domain.ErrCodeInvalid
	}
	u, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		return domain.ErrCodeInvalid
	}
	ok, err := s.repo.ConsumeSMSCode(ctx, u.PhoneHMAC, codeHash(u.PhoneHMAC, code))
	if err != nil {
		return err
	}
	if !ok {
		_ = s.pool.InTx(ctx, func(tx db.Tx) error {
			return s.repo.RecordAttempt(ctx, tx, u.ID, u.PhoneHMAC, false, "")
		})
		return domain.ErrCodeInvalid
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.RecordAttempt(ctx, tx, u.ID, u.PhoneHMAC, true, "")
	})
}

// plainPhone 解密账号手机号。
func (s *Service) plainPhone(u infra.User) (string, error) {
	if s.enc == nil {
		return string(u.PhoneEnc), nil
	}
	raw, err := s.enc.Decrypt(u.PhoneEnc)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// LoginResult 登录结果。
type LoginResult struct {
	UserID   int64
	IsNew    bool
	Tokens   contract.Tokens
	Nickname string
}

// WeChatLogin 微信登录注册一体化：code2session → 手机号 → 盲索引查/建 → 会话。
// identityType 仅在注册新用户时生效（1个人/2组织；0=默认个人），老用户忽略。
func (s *Service) WeChatLogin(ctx context.Context, code, phoneCode, device, ip string, identityType int16) (LoginResult, error) {
	if code == "" {
		return LoginResult{}, fmt.Errorf("identity: 缺少微信 code")
	}
	openID, _, err := s.wechat.Code2Session(ctx, code)
	if err != nil {
		return LoginResult{}, fmt.Errorf("%w: %v", domain.ErrWeChatUpstream, err)
	}
	if phoneCode == "" {
		return LoginResult{}, domain.ErrPhoneRequired
	}
	phone, err := s.wechat.GetPhoneNumberByCode(ctx, phoneCode)
	if err != nil {
		return LoginResult{}, fmt.Errorf("%w: %v", domain.ErrWeChatUpstream, err)
	}
	if !domain.ValidPhone(phone) {
		return LoginResult{}, domain.ErrPhoneRequired
	}
	hmac := sensitive.BlindIndex(s.blindKey, phone)

	u, err := s.repo.FindUserByPhoneHMAC(ctx, hmac)
	isNew := false
	switch {
	case err == infra.ErrNotFound:
		isNew = true
		if identityType == 0 {
			identityType = 1
		}
		if u, err = s.register(ctx, openID, phone, hmac, identityType); err != nil {
			return LoginResult{}, err
		}
	case err != nil:
		return LoginResult{}, err
	default:
		if !domain.CanLogin(u.Status) {
			return LoginResult{}, domain.StatusError(u.Status)
		}
		if u.OpenID != openID {
			if err := s.pool.InTx(ctx, func(tx db.Tx) error {
				return s.repo.BindOpenID(ctx, tx, u.ID, openID)
			}); err != nil {
				return LoginResult{}, err
			}
		}
	}

	tokens, err := s.startSession(ctx, u.ID, u.AuthzVer, device)
	if err != nil {
		return LoginResult{}, err
	}
	_ = s.repo.TouchActive(ctx, u.ID)
	return LoginResult{UserID: u.ID, IsNew: isNew, Tokens: tokens, Nickname: u.Nickname}, nil
}

// register 新建用户并发布 UserRegistered（同事务）。
func (s *Service) register(ctx context.Context, openID, phone string, hmac []byte, identityType int16) (infra.User, error) {
	var phoneEnc []byte
	if s.enc != nil {
		v, err := s.enc.Encrypt([]byte(phone))
		if err != nil {
			return infra.User{}, err
		}
		phoneEnc = v
	} else {
		phoneEnc = []byte(phone) // 未配置密钥（开发）：明文占位，启动时已告警
	}
	u := infra.User{OpenID: openID, PhoneEnc: phoneEnc, PhoneHMAC: hmac, Nickname: "猫平方用户", AuthzVer: 1, IdentityType: identityType}
	err := s.pool.InTx(ctx, func(tx db.Tx) error {
		id, err := s.repo.CreateUser(ctx, tx, u)
		if err != nil {
			return err
		}
		u.ID = id
		obs.Default().Inc("catcube_user_registered_total")
		if s.bus == nil {
			return nil
		}
		return s.bus.Publish(ctx, tx, contract.NewEvent(ids.New().String(), contract.EventUserRegistered, id))
	})
	if err != nil {
		return infra.User{}, err
	}
	return u, nil
}

// startSession 会话上限（≤3，最旧优先吊销）+ 新会话 + Access 签发。
func (s *Service) startSession(ctx context.Context, userID int64, authzVer int, device string) (contract.Tokens, error) {
	plain, hash, err := NewRefreshToken()
	if err != nil {
		return contract.Tokens{}, err
	}
	existing, err := s.repo.ActiveSessions(ctx, userID)
	if err != nil {
		return contract.Tokens{}, err
	}
	revoke := domain.SessionsToRevoke(existing, domain.MaxSessions)

	var sid int64
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.RevokeSessions(ctx, tx, revoke); err != nil {
			return err
		}
		var e error
		sid, e = s.repo.CreateSession(ctx, tx, userID, hash, device, RefreshTTL)
		return e
	})
	if err != nil {
		return contract.Tokens{}, err
	}
	access, err := s.tokens.IssueAccess(userID, sid, authzVer)
	if err != nil {
		return contract.Tokens{}, err
	}
	return contract.Tokens{AccessToken: access, RefreshToken: plain, ExpiresIn: int(s.tokens.AccessTTL().Seconds())}, nil
}

// IssueTokens 为已认证用户直接签发会话（后台口令登录）。
func (s *Service) IssueTokens(ctx context.Context, userID int64, device string) (contract.Tokens, error) {
	u, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		return contract.Tokens{}, err
	}
	if !domain.CanLogin(u.Status) {
		return contract.Tokens{}, domain.StatusError(u.Status)
	}
	return s.startSession(ctx, u.ID, u.AuthzVer, device)
}

// Refresh 刷新：校验 refresh 哈希 → 滚动轮换 → 新 Access（authz_ver 取当前值，消除陈旧窗口）。
func (s *Service) Refresh(ctx context.Context, refreshPlain string) (contract.Tokens, error) {
	if refreshPlain == "" {
		return contract.Tokens{}, domain.ErrRefreshInvalid
	}
	sum := sha256.Sum256([]byte(refreshPlain))
	sess, err := s.repo.SessionByRefreshHash(ctx, sum[:])
	if err == infra.ErrNotFound {
		return contract.Tokens{}, domain.ErrRefreshInvalid
	}
	if err != nil {
		return contract.Tokens{}, err
	}
	u, err := s.repo.UserByID(ctx, sess.UserID)
	if err != nil {
		return contract.Tokens{}, domain.ErrRefreshInvalid
	}
	if !domain.CanLogin(u.Status) {
		return contract.Tokens{}, domain.StatusError(u.Status)
	}
	newPlain, newHash, err := NewRefreshToken()
	if err != nil {
		return contract.Tokens{}, err
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.RotateSession(ctx, tx, sess.ID, newHash, RefreshTTL)
	}); err != nil {
		return contract.Tokens{}, err
	}
	access, err := s.tokens.IssueAccess(u.ID, sess.ID, u.AuthzVer)
	if err != nil {
		return contract.Tokens{}, err
	}
	return contract.Tokens{AccessToken: access, RefreshToken: newPlain, ExpiresIn: int(s.tokens.AccessTTL().Seconds())}, nil
}

// RequestSMSCode 发送验证码（同号历史未用码作废，5 分钟有效）。
func (s *Service) RequestSMSCode(ctx context.Context, phone string) error {
	if !domain.ValidPhone(phone) {
		return domain.ErrPhoneRequired
	}
	code := s.DevSMSCode
	if code == "" {
		code = randomCode()
	}
	hmac := sensitive.BlindIndex(s.blindKey, phone)
	if err := s.repo.SaveSMSCode(ctx, hmac, codeHash(hmac, code), SMSCodeTTL); err != nil {
		return err
	}
	if s.notify != nil {
		if err := s.notify.SendSMS(ctx, phone, "login", map[string]string{"code": code}); err != nil {
			return err
		}
		return nil
	}
	// 未配置短信通道（开发/联调）：验证码只进日志，绝不回传响应
	slog.Info("sms code (dev, 未配置短信通道)", "phone", maskTail(phone), "code", code)
	return nil
}

// SMSLogin 短信验证码登录（备选通道）：锁定 → 校验码 → 查/建用户 → 会话。
func (s *Service) SMSLogin(ctx context.Context, phone, code, device, ip string) (LoginResult, error) {
	if !domain.ValidPhone(phone) || code == "" {
		return LoginResult{}, domain.ErrPhoneRequired
	}
	hmac := sensitive.BlindIndex(s.blindKey, phone)
	fails, lastFail, err := s.repo.RecentFailures(ctx, hmac)
	if err != nil {
		return LoginResult{}, err
	}
	if domain.LockedN(fails, lastFail, time.Now(), s.threshold(ctx)) {
		return LoginResult{}, domain.ErrLocked
	}
	ok, err := s.repo.ConsumeSMSCode(ctx, hmac, codeHash(hmac, code))
	if err != nil {
		return LoginResult{}, err
	}
	if !ok {
		_ = s.pool.InTx(ctx, func(tx db.Tx) error {
			return s.repo.RecordAttempt(ctx, tx, 0, hmac, false, ip)
		})
		return LoginResult{}, domain.ErrCodeInvalid
	}

	u, err := s.repo.FindUserByPhoneHMAC(ctx, hmac)
	isNew := false
	switch {
	case err == infra.ErrNotFound:
		isNew = true
		if u, err = s.register(ctx, "", phone, hmac, 1); err != nil {
			return LoginResult{}, err
		}
	case err != nil:
		return LoginResult{}, err
	default:
		if !domain.CanLogin(u.Status) {
			return LoginResult{}, domain.StatusError(u.Status)
		}
	}
	tokens, err := s.startSession(ctx, u.ID, u.AuthzVer, device)
	if err != nil {
		return LoginResult{}, err
	}
	_ = s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.RecordAttempt(ctx, tx, u.ID, hmac, true, ip)
	})
	return LoginResult{UserID: u.ID, IsNew: isNew, Tokens: tokens, Nickname: u.Nickname}, nil
}

// randomCode 6 位数字验证码。
func randomCode() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	n := binary.BigEndian.Uint32(b[:]) % 1000000
	return fmt.Sprintf("%06d", n)
}

// codeHash 验证码哈希（与手机号盲索引绑定，防撞库/彩虹表）。
func codeHash(phoneHMAC []byte, code string) []byte {
	h := sha256.New()
	h.Write(phoneHMAC)
	h.Write([]byte(":"))
	h.Write([]byte(code))
	return h.Sum(nil)
}

func maskTail(phone string) string {
	if len(phone) < 4 {
		return "***"
	}
	return "***" + phone[len(phone)-4:]
}
