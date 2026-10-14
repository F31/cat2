// Package infra social 仓储（PG，soc_ 前缀）。
package infra

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/db"
	"catcube/internal/modules/social/internal/domain"
)

// Store social 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// Follow 关注关系。
type Follow struct {
	FollowerID int64
	Ref        domain.Ref
	CreatedAt  time.Time
}

// Favorite 收藏项。
type Favorite struct {
	ID        int64
	UserID    int64
	Ref       domain.Ref
	Kind      string
	CreatedAt time.Time
}

// InsertFollow 幂等关注。
func (s *Store) InsertFollow(ctx context.Context, tx db.Tx, followerID int64, ref domain.Ref, id int64) error {
	raw, _ := json.Marshal(ref)
	_, err := tx.Exec(ctx, `
		INSERT INTO soc_follow (id, follower_id, followee_ref, followee_type, followee_id, created_at)
		VALUES ($1,$2,$3,$4,$5,now())
		ON CONFLICT (follower_id, followee_type, followee_id) DO NOTHING`,
		id, followerID, raw, ref.Type, ref.ID)
	return err
}

// DeleteFollow 取关（返回是否删除）。
func (s *Store) DeleteFollow(ctx context.Context, followerID int64, ref domain.Ref) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM soc_follow WHERE follower_id=$1 AND followee_type=$2 AND followee_id=$3`,
		followerID, ref.Type, ref.ID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// IsFollowing 是否已关注。
func (s *Store) IsFollowing(ctx context.Context, followerID int64, ref domain.Ref) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(1) FROM soc_follow WHERE follower_id=$1 AND followee_type=$2 AND followee_id=$3`,
		followerID, ref.Type, ref.ID).Scan(&n)
	return n > 0, err
}

const followCols = `follower_id, COALESCE(followee_ref,'{}'::jsonb), created_at`

func scanFollow(row interface{ Scan(...any) error }) (Follow, error) {
	var f Follow
	var raw []byte
	if err := row.Scan(&f.FollowerID, &raw, &f.CreatedAt); err != nil {
		return f, err
	}
	_ = json.Unmarshal(raw, &f.Ref)
	return f, nil
}

// Following 我关注的（新→旧）。
func (s *Store) Following(ctx context.Context, followerID int64) ([]Follow, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+followCols+` FROM soc_follow WHERE follower_id=$1 ORDER BY created_at DESC`, followerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Follow
	for rows.Next() {
		f, err := scanFollow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FollowersOf 关注某目标的关注者 id 列表。
func (s *Store) FollowersOf(ctx context.Context, ref domain.Ref) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT follower_id FROM soc_follow WHERE followee_type=$1 AND followee_id=$2`, ref.Type, ref.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// InsertFavorite 幂等收藏。
func (s *Store) InsertFavorite(ctx context.Context, tx db.Tx, userID int64, ref domain.Ref, kind string, id int64) error {
	raw, _ := json.Marshal(ref)
	_, err := tx.Exec(ctx, `
		INSERT INTO soc_favorite (id, user_id, target_ref, target_type, target_id, kind, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,now())
		ON CONFLICT (user_id, target_type, target_id) DO NOTHING`,
		id, userID, raw, ref.Type, ref.ID, kind)
	return err
}

// DeleteFavorite 取消收藏（返回是否删除）。
func (s *Store) DeleteFavorite(ctx context.Context, userID int64, ref domain.Ref) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM soc_favorite WHERE user_id=$1 AND target_type=$2 AND target_id=$3`,
		userID, ref.Type, ref.ID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// Favorites 我的收藏（按 kind 过滤，新→旧）。
func (s *Store) Favorites(ctx context.Context, userID int64, kind string) ([]Favorite, error) {
	q := `SELECT id, user_id, COALESCE(target_ref,'{}'::jsonb), kind, created_at FROM soc_favorite WHERE user_id=$1`
	args := []any{userID}
	if kind != "" {
		q += ` AND kind=$2`
		args = append(args, kind)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Favorite
	for rows.Next() {
		var f Favorite
		var raw []byte
		if err := rows.Scan(&f.ID, &f.UserID, &raw, &f.Kind, &f.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &f.Ref)
		out = append(out, f)
	}
	return out, rows.Err()
}

// BumpUnread 未读 +1 并记录最近关注者摘要。
func (s *Store) BumpUnread(ctx context.Context, tx db.Tx, userID int64, brief []byte) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO soc_follow_unread (user_id, unread_count, last_follower_brief, updated_at)
		VALUES ($1,1,$2,now())
		ON CONFLICT (user_id) DO UPDATE SET unread_count=soc_follow_unread.unread_count+1,
			last_follower_brief=EXCLUDED.last_follower_brief, updated_at=now()`,
		userID, brief)
	return err
}

// Unread 未读数。
func (s *Store) Unread(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(unread_count,0) FROM soc_follow_unread WHERE user_id=$1`, userID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}
