// Package infra identity 仓储（PG，idn_ 前缀）。
package infra

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/db"
	"catcube/internal/modules/identity/internal/domain"
	"catcube/pkg/ids"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 记录不存在")

// User 用户记录（仅本模块 infra 可见；对外只暴露 contract.UserBrief）。
type User struct {
	ID             int64
	OpenID         string
	PhoneEnc       []byte
	PhoneHMAC      []byte
	Nickname       string
	AuthzVer       int
	Status         domain.UserStatus
	RealnameStatus int16
	IdentityType   int16
}

// Session 会话记录。
type Session struct {
	ID       int64
	UserID   int64
	ExpireAt time.Time
}

// Store identity 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// Pool 暴露连接池（module 层装配事务用）。
func (s *Store) Pool() *db.Pool { return s.pool }

const userCols = `id, COALESCE(wechat_openid,''), phone_enc, phone_hmac, COALESCE(nickname,''), authz_ver, status, realname_status, identity_type`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.OpenID, &u.PhoneEnc, &u.PhoneHMAC, &u.Nickname, &u.AuthzVer, &u.Status, &u.RealnameStatus, &u.IdentityType)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// FindUserByPhoneHMAC 按盲索引查用户（注册/登录一体化入口）。
func (s *Store) FindUserByPhoneHMAC(ctx context.Context, hmac []byte) (User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM idn_user WHERE phone_hmac=$1`, hmac))
}

// UserByID 按 ID 查用户。
func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM idn_user WHERE id=$1`, id))
}

// CreateUser 新建用户（与事件同事务，tx 由调用方提供）。
func (s *Store) CreateUser(ctx context.Context, tx db.Tx, u User) (int64, error) {
	id := int64(ids.New())
	_, err := tx.Exec(ctx, `
		INSERT INTO idn_user (id, wechat_openid, phone_enc, phone_hmac, nickname, status, identity_type, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,1,now(),now())`,
		id, u.OpenID, u.PhoneEnc, u.PhoneHMAC, u.Nickname, domain.StatusActive)
	return id, err
}

// BindOpenID 绑定/更新 openid（登录时发现的变更）。
func (s *Store) BindOpenID(ctx context.Context, tx db.Tx, userID int64, openID string) error {
	_, err := tx.Exec(ctx, `UPDATE idn_user SET wechat_openid=$2, updated_at=now() WHERE id=$1`, userID, openID)
	return err
}

// TouchActive 更新最近活跃时间。
func (s *Store) TouchActive(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE idn_user SET last_active_at=now() WHERE id=$1`, userID)
	return err
}

// ActiveSessions 活跃会话（按创建时间升序，供上限策略吊销最旧）。
func (s *Store) ActiveSessions(ctx context.Context, userID int64) ([]domain.SessionBrief, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, created_at FROM idn_session
		 WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>now() ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SessionBrief
	for rows.Next() {
		var b domain.SessionBrief
		if err := rows.Scan(&b.ID, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// RevokeSessions 批量吊销会话。
func (s *Store) RevokeSessions(ctx context.Context, tx db.Tx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE idn_session SET revoked_at=now() WHERE id = ANY($1)`, ids)
	return err
}

// CreateSession 新建会话（存 refresh 哈希，不存明文）。
func (s *Store) CreateSession(ctx context.Context, tx db.Tx, userID int64, refreshHash []byte, device string, ttl time.Duration) (int64, error) {
	id := int64(ids.New())
	_, err := tx.Exec(ctx, `
		INSERT INTO idn_session (id, user_id, refresh_token_hash, device_info, expires_at, created_at)
		VALUES ($1,$2,$3,$4,now()+$5::interval,now())`,
		id, userID, refreshHash, device, ttl.String())
	return id, err
}

// SessionByRefreshHash 按 refresh 哈希查有效会话。
func (s *Store) SessionByRefreshHash(ctx context.Context, hash []byte) (Session, error) {
	var sess Session
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, expires_at FROM idn_session
		 WHERE refresh_token_hash=$1 AND revoked_at IS NULL AND expires_at>now()`, hash).
		Scan(&sess.ID, &sess.UserID, &sess.ExpireAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sess, ErrNotFound
	}
	return sess, err
}

// RotateSession 轮换 refresh 哈希并续期（滚动 7 天）。
func (s *Store) RotateSession(ctx context.Context, tx db.Tx, sessionID int64, newHash []byte, ttl time.Duration) error {
	_, err := tx.Exec(ctx, `
		UPDATE idn_session SET refresh_token_hash=$2, expires_at=now()+$3::interval WHERE id=$1`,
		sessionID, newHash, ttl.String())
	return err
}

// RecordAttempt 记录登录尝试（成功/失败）。
func (s *Store) RecordAttempt(ctx context.Context, tx db.Tx, userID int64, phoneHMAC []byte, success bool, ip string) error {
	var uid any
	if userID != 0 {
		uid = userID
	}
	var peer any
	if ip != "" {
		peer = ip
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO idn_login_attempt (user_id, phone_hmac, success, ip, created_at)
		VALUES ($1,$2,$3,$4,now())`, uid, phoneHMAC, success, peer)
	return err
}

// RecentFailures 窗口内失败次数与最近一次失败时间。
func (s *Store) RecentFailures(ctx context.Context, phoneHMAC []byte) (int, time.Time, error) {
	var n int
	var last *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT count(1), max(created_at) FROM idn_login_attempt
		WHERE phone_hmac=$1 AND success=false AND created_at > now() - $2::interval`,
		phoneHMAC, domain.FailWindow.String()).Scan(&n, &last)
	if err != nil {
		return 0, time.Time{}, err
	}
	if last == nil {
		return n, time.Time{}, nil
	}
	return n, *last, nil
}

// SaveSMSCode 保存验证码哈希（同手机号历史未用码即作废）。
func (s *Store) SaveSMSCode(ctx context.Context, phoneHMAC, codeHash []byte, ttl time.Duration) error {
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE idn_sms_code SET used_at=now() WHERE phone_hmac=$1 AND used_at IS NULL`, phoneHMAC); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO idn_sms_code (phone_hmac, code_hash, expires_at, created_at)
			VALUES ($1,$2,now()+$3::interval,now())`, phoneHMAC, codeHash, ttl.String())
		return err
	})
}

// ConsumeSMSCode 校验并作废验证码（原子：命中即置 used_at）。
func (s *Store) ConsumeSMSCode(ctx context.Context, phoneHMAC, codeHash []byte) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE idn_sms_code SET used_at=now()
		WHERE phone_hmac=$1 AND code_hash=$2 AND used_at IS NULL AND expires_at>now()`, phoneHMAC, codeHash)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// LogDeactivation 注销留痕（原因）。
func (s *Store) LogDeactivation(ctx context.Context, tx db.Tx, userID int64, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO idn_deactivation_log (user_id, reason, created_at) VALUES ($1,$2,now())`,
		userID, reason)
	return err
}
