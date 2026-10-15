// Package infra circle 仓储（PG，cir_ 前缀）。
package infra

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"catcube/internal/kernel/db"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 记录不存在")

// Querier pgx 查询接口（Pool 与 Tx 均满足）。
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Circle 圈子行。
type Circle struct {
	ID             int64
	Name           string
	Intro          string
	CoverFileID    int64
	RegionCode     string
	Industry       []string
	Type           int16
	DurationDays   int16
	EndAt          *time.Time
	OwnerUserID    int64
	OrgID          int64
	MemberCount    int
	RecommendableAt time.Time
	Status         int16
	CreatedAt      time.Time
}

// Member 成员行。
type Member struct {
	ID        int64
	CircleID  int64
	UserID    int64
	JoinedVia int16
	Status    int16
}

// Demand 需求行。
type Demand struct {
	ID            int64
	CircleID      int64
	AuthorID      int64
	Title         string
	Content       string
	DemandType    string
	RegionCode    string
	Industry      []string
	ExpireAt      time.Time
	Scope         int16
	Status        int16
	CreatedAt     time.Time
}

// Store circle 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

const circleCols = `id, name, COALESCE(intro,''), COALESCE(cover_file_id,0), COALESCE(region_code,''),
	industry_codes, type, duration_days, end_at, owner_user_id, COALESCE(org_id,0), member_count,
	recommendable_at, status, created_at`

func scanCircle(row pgx.Row) (Circle, error) {
	var c Circle
	var ind []byte
	err := row.Scan(&c.ID, &c.Name, &c.Intro, &c.CoverFileID, &c.RegionCode, &ind, &c.Type, &c.DurationDays,
		&c.EndAt, &c.OwnerUserID, &c.OrgID, &c.MemberCount, &c.RecommendableAt, &c.Status, &c.CreatedAt)
	if err != nil {
		return Circle{}, err
	}
	_ = json.Unmarshal(ind, &c.Industry)
	return c, nil
}

func scanCircles(rows pgx.Rows) ([]Circle, error) {
	var out []Circle
	for rows.Next() {
		c, err := scanCircle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateCircle 新建圈子。
func (s *Store) CreateCircle(ctx context.Context, q Querier, c Circle) error {
	ind, _ := json.Marshal(c.Industry)
	_, err := q.Exec(ctx, `
		INSERT INTO cir_circle (id, name, intro, cover_file_id, region_code, industry_codes, type,
			duration_days, end_at, owner_user_id, org_id, member_count, recommendable_at, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,1,now()+interval '7 days',0)`,
		c.ID, c.Name, nullStr(c.Intro), nullID(c.CoverFileID), nullStr(c.RegionCode), ind, c.Type,
		c.DurationDays, c.EndAt, c.OwnerUserID, nullID(c.OrgID))
	return err
}

// GetCircle 取圈子。
func (s *Store) GetCircle(ctx context.Context, id int64) (Circle, error) {
	c, err := scanCircle(s.pool.QueryRow(ctx, `SELECT `+circleCols+` FROM cir_circle WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Circle{}, ErrNotFound
	}
	return c, err
}

// ListByUser 我加入/创建的圈子。
func (s *Store) ListByUser(ctx context.Context, userID int64) ([]Circle, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+circleCols+` FROM cir_circle c
		WHERE c.owner_user_id=$1 OR EXISTS (SELECT 1 FROM cir_member m WHERE m.circle_id=c.id AND m.user_ref=$1 AND m.status=0)
		ORDER BY c.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCircles(rows)
}

// CountCreatedToday 当日建圈数（配额：每人每日 ≤1）。
func (s *Store) CountCreatedToday(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(1) FROM cir_circle WHERE owner_user_id=$1 AND created_at >= date_trunc('day', now())`, userID).Scan(&n)
	return n, err
}

// CountOwned 当前持有（进行中）圈子数（配额：同时最多 3 个）。
func (s *Store) CountOwned(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(1) FROM cir_circle WHERE owner_user_id=$1 AND status=0`, userID).Scan(&n)
	return n, err
}

// SetStatus 变更圈子状态。
func (s *Store) SetStatus(ctx context.Context, q Querier, id int64, status int16, dissolved bool) error {
	_, err := q.Exec(ctx, `UPDATE cir_circle SET status=$2,
		dissolved_at=CASE WHEN $3 THEN now() ELSE dissolved_at END, version=version+1, updated_at=now() WHERE id=$1`,
		id, status, dissolved)
	return err
}

// ConvertPermanent 转常设（清空 end_at、类型=常设）。
func (s *Store) ConvertPermanent(ctx context.Context, q Querier, id int64) error {
	_, err := q.Exec(ctx, `UPDATE cir_circle SET type=1, end_at=NULL, status=0, version=version+1, updated_at=now() WHERE id=$1`, id)
	return err
}

// AddMember 加入成员（幂等）。
func (s *Store) AddMember(ctx context.Context, q Querier, id, circleID, userID int64, via int16) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO cir_member (id, circle_id, user_ref, joined_via, status)
		VALUES ($1,$2,$3,$4,0) ON CONFLICT (circle_id, user_ref) DO UPDATE SET status=0 WHERE cir_member.status<>0`,
		id, circleID, userID, via)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		_, _ = q.Exec(ctx, `UPDATE cir_circle SET member_count=member_count+1 WHERE id=$1`, circleID)
	}
	return tag.RowsAffected() > 0, nil
}

// IsMember 是否成员。
func (s *Store) IsMember(ctx context.Context, circleID, userID int64) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(1) FROM cir_member WHERE circle_id=$1 AND user_ref=$2 AND status=0`, circleID, userID).Scan(&n)
	return n > 0, err
}

// ListMembers 成员 ID 列表。
func (s *Store) ListMembers(ctx context.Context, circleID int64) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_ref FROM cir_member WHERE circle_id=$1 AND status=0 ORDER BY joined_at`, circleID)
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

// InsertInvite 新建邀请码（同一时间单圈单有效码）。
func (s *Store) InsertInvite(ctx context.Context, q Querier, id, circleID, createdBy int64, code string, expire time.Time) error {
	if _, err := q.Exec(ctx, `UPDATE cir_invite SET status=1 WHERE circle_id=$1 AND status=0`, circleID); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `
		INSERT INTO cir_invite (id, circle_id, code, created_by, expire_at, status)
		VALUES ($1,$2,$3,$4,$5,0)`, id, circleID, code, createdBy, expire)
	return err
}

// GetInvite 取有效邀请码。
func (s *Store) GetInvite(ctx context.Context, code string) (circleID int64, expire time.Time, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT circle_id, expire_at FROM cir_invite WHERE code=$1 AND status=0`, code).Scan(&circleID, &expire)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, time.Time{}, ErrNotFound
	}
	return circleID, expire, err
}

// ---- 需求 ----

const demandCols = `id, circle_id, author_id, title, content, COALESCE(demand_type,''), COALESCE(region_code,''),
	industry_codes, expire_at, scope, status, created_at`

func scanDemand(row pgx.Row) (Demand, error) {
	var d Demand
	var ind []byte
	err := row.Scan(&d.ID, &d.CircleID, &d.AuthorID, &d.Title, &d.Content, &d.DemandType, &d.RegionCode,
		&ind, &d.ExpireAt, &d.Scope, &d.Status, &d.CreatedAt)
	if err != nil {
		return Demand{}, err
	}
	_ = json.Unmarshal(ind, &d.Industry)
	return d, nil
}

// CreateDemand 发布需求。
func (s *Store) CreateDemand(ctx context.Context, q Querier, d Demand, contactEnc []byte) error {
	ind, _ := json.Marshal(d.Industry)
	_, err := q.Exec(ctx, `
		INSERT INTO cir_demand (id, circle_id, author_id, title, content, demand_type, region_code,
			industry_codes, expire_at, scope, contact_phone_enc, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,0)`,
		d.ID, d.CircleID, d.AuthorID, d.Title, d.Content, nullStr(d.DemandType), nullStr(d.RegionCode),
		ind, d.ExpireAt, d.Scope, contactEnc)
	return err
}

// GetDemand 取需求。
func (s *Store) GetDemand(ctx context.Context, id int64) (Demand, error) {
	d, err := scanDemand(s.pool.QueryRow(ctx, `SELECT `+demandCols+` FROM cir_demand WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Demand{}, ErrNotFound
	}
	return d, err
}

// ListDemands 圈内需求（有效在前，新→旧）。
func (s *Store) ListDemands(ctx context.Context, circleID int64, limit int) ([]Demand, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `SELECT `+demandCols+` FROM cir_demand
		WHERE circle_id=$1 AND status<>2 ORDER BY status, created_at DESC LIMIT $2`, circleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Demand
	for rows.Next() {
		d, err := scanDemand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CountUserPostsToday 当日个人圈内发布数（跨圈合计 3 条）。
func (s *Store) CountUserPostsToday(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(1) FROM cir_demand WHERE author_id=$1 AND created_at >= date_trunc('day', now()))
		     + (SELECT count(1) FROM cir_feed WHERE author_id=$1 AND is_announcement=false AND created_at >= date_trunc('day', now()))`,
		userID).Scan(&n)
	return n, err
}

// CountAnnounceToday 当日该圈公告数。
func (s *Store) CountAnnounceToday(ctx context.Context, circleID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(1) FROM cir_feed WHERE circle_id=$1 AND is_announcement=true AND created_at >= date_trunc('day', now())`, circleID).Scan(&n)
	return n, err
}

// RenewDemand 续期 30 天。
func (s *Store) RenewDemand(ctx context.Context, q Querier, id int64, expire time.Time) error {
	_, err := q.Exec(ctx, `UPDATE cir_demand SET expire_at=$2, status=0 WHERE id=$1`, id, expire)
	return err
}

// ExpireDueDemands 需求到期转「已过期」（SchedulerJob）。
func (s *Store) ExpireDueDemands(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE cir_demand SET status=1 WHERE id IN (
			SELECT id FROM cir_demand WHERE status=0 AND expire_at<now() ORDER BY expire_at LIMIT $1
		) RETURNING id, circle_id`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id, cid int64
		if err := rows.Scan(&id, &cid); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CreateFeed 发布动态/公告。
func (s *Store) CreateFeed(ctx context.Context, q Querier, id, circleID, authorID int64, text string, images []byte, announce bool) error {
	_, err := q.Exec(ctx, `
		INSERT INTO cir_feed (id, circle_id, author_id, text, image_file_ids, is_announcement, status)
		VALUES ($1,$2,$3,$4,$5,$6,0)`, id, circleID, authorID, nullStr(text), images, announce)
	if err == nil {
		_, _ = q.Exec(ctx, `UPDATE cir_circle SET feed_count=feed_count+1 WHERE id=$1`, circleID)
	}
	return err
}

// ListDueCircles 到期未处理的临时圈（SchedulerJob）。
func (s *Store) ListDueCircles(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM cir_circle WHERE type=2 AND status=0 AND end_at IS NOT NULL AND end_at<now() ORDER BY end_at LIMIT $1`, limit)
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

func nullID(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}
