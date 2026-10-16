// Package infra notice 仓储（PG，ntc_ 前缀）。
package infra

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"catcube/internal/kernel/db"
	"catcube/internal/modules/notice/internal/domain"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 记录不存在")

// Querier pgx 查询接口。
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Notice 站内信行。
type Notice struct {
	ID           int64
	UserID       int64
	TemplateCode string
	Title        string
	Content      string
	Ref          string
	Channel      string
	IsRead       bool
	ReadAt       *time.Time
	CreatedAt    time.Time
}

// Store notice 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// UpsertTemplate 模板登记（幂等；后台可改文案/渠道）。
func (s *Store) UpsertTemplate(ctx context.Context, id int64, t domain.Template) error {
	ch, _ := json.Marshal(t.Channels)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ntc_template (id, module, event_code, title_tpl, content_tpl, channels, receiver_rule, merge_rule, closeable, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,1)
		ON CONFLICT (event_code) DO NOTHING`,
		id, t.Module, t.EventCode, t.TitleTpl, t.ContentTpl, ch, t.ReceiverRule, t.MergeRule, t.Closeable)
	return err
}

// ListTemplates 模板列表。
func (s *Store) ListTemplates(ctx context.Context) ([]domain.Template, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT event_code, module, title_tpl, content_tpl, channels, receiver_rule, COALESCE(merge_rule,''), closeable
		FROM ntc_template WHERE status=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Template
	for rows.Next() {
		var t domain.Template
		var ch []byte
		if err := rows.Scan(&t.EventCode, &t.Module, &t.TitleTpl, &t.ContentTpl, &ch, &t.ReceiverRule, &t.MergeRule, &t.Closeable); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(ch, &t.Channels)
		t.Code = t.EventCode
		out = append(out, t)
	}
	return out, rows.Err()
}

// InsertNotice 写站内信。
func (s *Store) InsertNotice(ctx context.Context, q Querier, n Notice) error {
	_, err := q.Exec(ctx, `
		INSERT INTO ntc_notice (id, user_id, template_code, title, content, ref, channel, is_read)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,false)`,
		n.ID, n.UserID, n.TemplateCode, n.Title, n.Content, n.Ref, n.Channel)
	return err
}

// BatchInsertNotice 批量写站内信（定向公告；单次上限由调用方控制）。
func (s *Store) BatchInsertNotice(ctx context.Context, items []Notice) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	n := 0
	err := s.pool.InTx(ctx, func(tx db.Tx) error {
		for _, it := range items {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ntc_notice (id, user_id, template_code, title, content, ref, channel, is_read)
				VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,false)`,
				it.ID, it.UserID, it.TemplateCode, it.Title, it.Content, it.Ref, it.Channel); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// ListNotices 站内信列表。
func (s *Store) ListNotices(ctx context.Context, userID int64, unreadOnly bool, limit int) ([]Notice, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, template_code, title, content, COALESCE(ref,''), channel, is_read, read_at, created_at
		FROM ntc_notice WHERE user_id=$1 AND ($2=false OR is_read=false)
		ORDER BY created_at DESC LIMIT $3`, userID, unreadOnly, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notice
	for rows.Next() {
		var n Notice
		if err := rows.Scan(&n.ID, &n.UserID, &n.TemplateCode, &n.Title, &n.Content, &n.Ref, &n.Channel, &n.IsRead, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnreadCount 未读数。
func (s *Store) UnreadCount(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(1) FROM ntc_notice WHERE user_id=$1 AND is_read=false`, userID).Scan(&n)
	return n, err
}

// MarkRead 标记已读。
func (s *Store) MarkRead(ctx context.Context, userID, noticeID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE ntc_notice SET is_read=true, read_at=now() WHERE id=$1 AND user_id=$2`, noticeID, userID)
	return err
}

// Preference 读取单条偏好（缺省 enabled）。
func (s *Store) Preference(ctx context.Context, userID int64, code string) (bool, error) {
	var enabled bool
	err := s.pool.QueryRow(ctx, `SELECT enabled FROM ntc_preference WHERE user_id=$1 AND template_code=$2`, userID, code).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return enabled, err
}

// SetPreference 写偏好。
func (s *Store) SetPreference(ctx context.Context, userID int64, code string, enabled bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ntc_preference (user_id, template_code, enabled, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (user_id, template_code) DO UPDATE SET enabled=EXCLUDED.enabled, updated_at=now()`,
		userID, code, enabled)
	return err
}

// IncrQuota 申请授权额度（+n）。
func (s *Store) IncrQuota(ctx context.Context, userID int64, code string, n int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ntc_subscribe_quota (user_id, template_code, remaining, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (user_id, template_code) DO UPDATE SET remaining=ntc_subscribe_quota.remaining+$3, updated_at=now()`,
		userID, code, n)
	return err
}

// ConsumeQuota 消耗额度（有余额才 -1）；返回是否消耗成功。
func (s *Store) ConsumeQuota(ctx context.Context, q Querier, userID int64, code string) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE ntc_subscribe_quota SET remaining=remaining-1, updated_at=now()
		WHERE user_id=$1 AND template_code=$2 AND remaining>0`, userID, code)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// UpsertDigest 同圈同日合并计数（避免先判断再自增竞态）。
func (s *Store) UpsertDigest(ctx context.Context, circleID int64) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO ntc_digest (circle_id, date, count, updated_at)
		VALUES ($1, current_date, 1, now())
		ON CONFLICT (circle_id, date) DO UPDATE SET count=ntc_digest.count+1, updated_at=now()
		RETURNING count`, circleID).Scan(&count)
	return count, err
}

// DueDigests 当日待日结的合并项。
func (s *Store) DueDigests(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT circle_id FROM ntc_digest WHERE date=current_date AND notice_id IS NULL ORDER BY circle_id LIMIT $1`, limit)
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

// DigestCount 取合并计数。
func (s *Store) DigestCount(ctx context.Context, circleID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count FROM ntc_digest WHERE circle_id=$1 AND date=current_date`, circleID).Scan(&n)
	return n, err
}

// MarkDigestSent 标记合并已发送。
func (s *Store) MarkDigestSent(ctx context.Context, circleID, noticeID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE ntc_digest SET notice_id=$2, updated_at=now() WHERE circle_id=$1 AND date=current_date`, circleID, noticeID)
	return err
}
