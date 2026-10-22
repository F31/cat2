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
	RealNameEnc    []byte
	IDCardNoEnc    []byte
	RegionCode     string
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

const userCols = `id, COALESCE(wechat_openid,''), phone_enc, phone_hmac, COALESCE(nickname,''),
	id_card_name_enc, id_card_no_enc, COALESCE(region_code,''), authz_ver, status, realname_status, identity_type`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.OpenID, &u.PhoneEnc, &u.PhoneHMAC, &u.Nickname,
		&u.RealNameEnc, &u.IDCardNoEnc, &u.RegionCode, &u.AuthzVer, &u.Status, &u.RealnameStatus, &u.IdentityType)
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

// AdminUser 后台用户列表项。
type AdminUser struct {
	ID             int64
	Nickname       string
	Status         int16
	IdentityType   int16
	RealnameStatus int16
	PhoneEnc       []byte
	RealNameEnc    []byte
	IDCardNoEnc    []byte
	RegionCode     string
	CreatedAt      time.Time
}

// AdminListUsers 后台用户列表（status<0 表示全部；q 模糊匹配昵称）。
func (s *Store) AdminListUsers(ctx context.Context, status int16, q string, limit int) ([]AdminUser, error) {
	// 仅按状态/昵称检索：其余维度置「不限」（-1），避免默认值变成实际过滤条件
	return s.AdminSearchUsers(ctx, AdminUserFilter{Status: status, Q: q, Limit: limit, RealnameState: -1, IdentityType: -1})
}

// AdminUserFilter 后台用户检索条件（p4 §4.2）。
type AdminUserFilter struct {
	Status        int16  // -1 全部
	Q             string // 昵称模糊
	PhoneHMAC     []byte // 手机号盲索引精确
	RealnameState int16  // -1 全部
	IdentityType  int16  // -1 全部
	FromUnix      int64
	ToUnix        int64
	Limit         int
	Offset        int
}

// AdminSearchUsers 组合筛选检索。
func (s *Store) AdminSearchUsers(ctx context.Context, f AdminUserFilter) ([]AdminUser, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, COALESCE(nickname,''), status, identity_type, COALESCE(realname_status,0), phone_enc,
		       id_card_name_enc, id_card_no_enc, COALESCE(region_code,''), created_at
		  FROM idn_user
		 WHERE ($1 < 0 OR status=$1)
		   AND ($2 = '' OR nickname ILIKE '%'||$2||'%')
		   AND ($3::bytea IS NULL OR phone_hmac=$3)
		   AND ($4 = -1 OR realname_status=$4)
		   AND ($5 = -1 OR identity_type=$5)
		   AND ($6 = 0 OR created_at >= to_timestamp($6))
		   AND ($7 = 0 OR created_at < to_timestamp($7))
		 ORDER BY created_at DESC NULLS LAST, id DESC
		 LIMIT $8 OFFSET $9`,
		f.Status, f.Q, f.PhoneHMAC, f.RealnameState, f.IdentityType, f.FromUnix, f.ToUnix, f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminUser
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.ID, &u.Nickname, &u.Status, &u.IdentityType, &u.RealnameStatus, &u.PhoneEnc,
			&u.RealNameEnc, &u.IDCardNoEnc, &u.RegionCode, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetStatus 后台封禁/解封。
func (s *Store) SetStatus(ctx context.Context, userID int64, status int16) error {
	_, err := s.pool.Exec(ctx, `UPDATE idn_user SET status=$2, updated_at=now() WHERE id=$1`, userID, status)
	return err
}

// CreateUser 新建用户（与事件同事务，tx 由调用方提供）。
func (s *Store) CreateUser(ctx context.Context, tx db.Tx, u User) (int64, error) {
	id := int64(ids.New())
	identityType := u.IdentityType
	if identityType == 0 {
		identityType = 1
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO idn_user (id, wechat_openid, phone_enc, phone_hmac, nickname, status, identity_type, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,now(),now())`,
		id, u.OpenID, u.PhoneEnc, u.PhoneHMAC, u.Nickname, domain.StatusActive, identityType)
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

// FailuresSinceLastSuccess 自最近一次成功登录以来的失败次数（后台二次验证判定，p4 §3.4）。
func (s *Store) FailuresSinceLastSuccess(ctx context.Context, phoneHMAC []byte) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(1) FROM idn_login_attempt a
		WHERE a.phone_hmac=$1 AND a.success=false
		  AND a.created_at > COALESCE((SELECT max(created_at) FROM idn_login_attempt
		      WHERE phone_hmac=$1 AND success=true), 'epoch'::timestamptz)`, phoneHMAC).Scan(&n)
	return n, err
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
