// Package infra membership 仓储（PG，mbr_ 前缀）。
package infra

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/db"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 记录不存在")

// Member 会员记录。
type Member struct {
	UserID        int64
	Level         int16
	Status        int16
	StartedAt     time.Time
	ExpiresAt     time.Time
	LastOrderID   int64
	PriceSnapshot []byte
}

// SubAccount 子账号记录。
type SubAccount struct {
	ID        int64
	MasterID  int64
	SubUserID int64
	Status    int16
	InvitedAt time.Time
}

// Store membership 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// GetMember 取会员。
func (s *Store) GetMember(ctx context.Context, userID int64) (Member, error) {
	var m Member
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, level, status, started_at, expires_at, COALESCE(last_order_id,0), price_snapshot FROM mbr_member WHERE user_id=$1`, userID).
		Scan(&m.UserID, &m.Level, &m.Status, &m.StartedAt, &m.ExpiresAt, &m.LastOrderID, &m.PriceSnapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// UpsertMember 开通/续期（保留首次 started_at）。
func (s *Store) UpsertMember(ctx context.Context, tx db.Tx, m Member) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO mbr_member (user_id, level, status, started_at, expires_at, last_order_id, price_snapshot, created_at, updated_at)
		VALUES ($1,$2,1,now(),$3,$4,$5,now(),now())
		ON CONFLICT (user_id) DO UPDATE SET
			level=EXCLUDED.level, status=1, expires_at=EXCLUDED.expires_at,
			last_order_id=EXCLUDED.last_order_id, price_snapshot=EXCLUDED.price_snapshot, updated_at=now()`,
		m.UserID, m.Level, m.ExpiresAt, nullID(m.LastOrderID), snap(m.PriceSnapshot))
	return err
}

// ListExpired 到期但未标记过期的会员。
func (s *Store) ListExpired(ctx context.Context, limit int) ([]Member, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT user_id, level, status, started_at, expires_at, COALESCE(last_order_id,0), price_snapshot
		 FROM mbr_member WHERE status=1 AND expires_at < now() ORDER BY expires_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Level, &m.Status, &m.StartedAt, &m.ExpiresAt, &m.LastOrderID, &m.PriceSnapshot); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkExpired 置已过期。
func (s *Store) MarkExpired(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE mbr_member SET status=2, updated_at=now() WHERE user_id=$1 AND status=1`, userID)
	return err
}

// AdminList 后台会员列表（level<0/status<0 表示不过滤）。
func (s *Store) AdminList(ctx context.Context, level, status int16, limit int) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, level, status, started_at, expires_at, COALESCE(last_order_id,0), price_snapshot
		  FROM mbr_member
		 WHERE ($1 < 0 OR level=$1) AND ($2 < 0 OR status=$2)
		 ORDER BY expires_at DESC, user_id DESC
		 LIMIT $3`, level, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Level, &m.Status, &m.StartedAt, &m.ExpiresAt, &m.LastOrderID, &m.PriceSnapshot); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListSubs 子账号列表。
func (s *Store) ListSubs(ctx context.Context, masterID int64) ([]SubAccount, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, master_id, sub_user_id, status, invited_at FROM mbr_sub_account WHERE master_id=$1 ORDER BY invited_at`, masterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubAccount
	for rows.Next() {
		var a SubAccount
		if err := rows.Scan(&a.ID, &a.MasterID, &a.SubUserID, &a.Status, &a.InvitedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountActiveSubs 生效子账号数。
func (s *Store) CountActiveSubs(ctx context.Context, masterID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(1) FROM mbr_sub_account WHERE master_id=$1 AND status=1`, masterID).Scan(&n)
	return n, err
}

// InsertSub 邀请子账号（复活已存在记录）。
func (s *Store) InsertSub(ctx context.Context, tx db.Tx, id, masterID, subUserID int64, expiresAt time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO mbr_sub_account (id, master_id, sub_user_id, status, invited_at, expires_at)
		VALUES ($1,$2,$3,1,now(),$4)
		ON CONFLICT (master_id, sub_user_id) DO UPDATE SET status=1, removed_at=NULL, invited_at=now(), expires_at=EXCLUDED.expires_at`,
		id, masterID, subUserID, expiresAt)
	return err
}

// RemoveSub 移除子账号。
func (s *Store) RemoveSub(ctx context.Context, tx db.Tx, masterID, subUserID int64) error {
	_, err := tx.Exec(ctx,
		`UPDATE mbr_sub_account SET status=2, removed_at=now() WHERE master_id=$1 AND sub_user_id=$2 AND status=1`,
		masterID, subUserID)
	return err
}

// ExpireSubsByMaster 主账号企业版过期 → 子账号同步回落（status=2，保留归属记录）。
func (s *Store) ExpireSubsByMaster(ctx context.Context, masterID int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE mbr_sub_account SET status=2, removed_at=now() WHERE master_id=$1 AND status=1`, masterID)
	return err
}

// PurgeRemovedSubs 清理超过保留期（30 天）的已移除归属记录。
func (s *Store) PurgeRemovedSubs(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM mbr_sub_account WHERE status=2 AND removed_at IS NOT NULL AND removed_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func snap(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}
func nullID(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
