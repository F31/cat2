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
	ID              int64
	Name            string
	Intro           string
	CoverFileID     int64
	RegionCode      string
	Industry        []string
	Type            int16
	DurationDays    int16
	EndAt           *time.Time
	OwnerUserID     int64
	OrgID           int64
	MemberCount     int
	RecommendableAt time.Time
	Status          int16
	SourceRef       string
	CreatedAt       time.Time
}

// Member 成员行。
type Member struct {
	ID        int64
	CircleID  int64
	UserID    int64
	JoinedVia int16
	Status    int16
}

// MemberDetail 成员展示行。
type MemberDetail struct {
	UserID    int64
	JoinedVia int16
	JoinedAt  time.Time
}

// Application 入圈申请行。
type Application struct {
	ID          int64
	CircleID    int64
	ApplicantID int64
	Status      int16 // 0待审/1已通过/2已拒绝
	Reason      string
	CreatedAt   time.Time
}

// Feed 圈内动态行。
type Feed struct {
	ID             int64
	CircleID       int64
	AuthorID       int64
	Text           string
	ImageFileIDs   []int64
	IsAnnouncement bool
	CreatedAt      time.Time
}

// Demand 需求行。
type Demand struct {
	ID         int64
	CircleID   int64
	AuthorID   int64
	Title      string
	Content    string
	DemandType string
	RegionCode string
	Industry   []string
	ExpireAt   time.Time
	Scope      int16
	Status     int16
	CreatedAt  time.Time
}

// Store circle 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

const circleCols = `id, name, COALESCE(intro,''), COALESCE(cover_file_id,0), COALESCE(region_code,''),
	industry_codes, type, duration_days, end_at, owner_user_id, COALESCE(org_id,0), member_count,
	recommendable_at, status, COALESCE(source_ref,''), created_at`

func scanCircle(row pgx.Row) (Circle, error) {
	var c Circle
	var ind []byte
	err := row.Scan(&c.ID, &c.Name, &c.Intro, &c.CoverFileID, &c.RegionCode, &ind, &c.Type, &c.DurationDays,
		&c.EndAt, &c.OwnerUserID, &c.OrgID, &c.MemberCount, &c.RecommendableAt, &c.Status, &c.SourceRef, &c.CreatedAt)
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
			duration_days, end_at, owner_user_id, org_id, member_count, recommendable_at, status, source_ref)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,1,now()+interval '7 days',0,$12)`,
		c.ID, c.Name, nullStr(c.Intro), nullID(c.CoverFileID), nullStr(c.RegionCode), ind, c.Type,
		c.DurationDays, c.EndAt, c.OwnerUserID, nullID(c.OrgID), nullStr(c.SourceRef))
	return err
}

// GetBySource 按来源引用查圈子（活动自动建圈幂等）。
func (s *Store) GetBySource(ctx context.Context, sourceRef string) (Circle, error) {
	c, err := scanCircle(s.pool.QueryRow(ctx, `SELECT `+circleCols+` FROM cir_circle WHERE source_ref=$1`, sourceRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return Circle{}, ErrNotFound
	}
	return c, err
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
		WHERE (c.owner_user_id=$1 OR EXISTS (SELECT 1 FROM cir_member m WHERE m.circle_id=c.id AND m.user_ref=$1 AND m.status=0))
		  AND c.status NOT IN (3,4)
		ORDER BY c.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCircles(rows)
}

// ListDiscover 公开圈子发现（仅进行中；支持名称/区域/类型筛选）。
func (s *Store) ListDiscover(ctx context.Context, q, region string, typ int16, sort string, limit, offset int) ([]Circle, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	order := `c.member_count DESC, c.created_at DESC`
	switch sort {
	case "new":
		order = `c.created_at DESC`
	case "recommend":
		order = `CASE WHEN c.recommendable_at <= now() THEN 0 ELSE 1 END, c.member_count DESC, c.created_at DESC`
	}
	rows, err := s.pool.Query(ctx, `SELECT `+circleCols+` FROM cir_circle c
		WHERE c.status=0
		  AND ($1 = '' OR c.name ILIKE '%' || $1 || '%' OR COALESCE(c.intro,'') ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR c.region_code=$2)
		  AND ($3 = 0 OR c.type=$3)
		ORDER BY `+order+` LIMIT $4 OFFSET $5`, q, region, typ, limit, offset)
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

// ListMemberDetails 成员展示列表（含加入时间/途径）。
func (s *Store) ListMemberDetails(ctx context.Context, circleID int64) ([]MemberDetail, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_ref, joined_via, joined_at FROM cir_member
		WHERE circle_id=$1 AND status=0 ORDER BY joined_at`, circleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MemberDetail, 0)
	for rows.Next() {
		var m MemberDetail
		if err := rows.Scan(&m.UserID, &m.JoinedVia, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ApplicationStatus 我的申请状态（0待审/1通过/2拒绝；无记录 found=false）。
func (s *Store) ApplicationStatus(ctx context.Context, circleID, userID int64) (int16, bool, error) {
	var st int16
	err := s.pool.QueryRow(ctx, `SELECT status FROM cir_apply WHERE circle_id=$1 AND applicant_id=$2`,
		circleID, userID).Scan(&st)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return st, err == nil, err
}

// UpsertApplication 提交/重新提交入圈申请（已通过或待审幂等；被拒后可重申）。
func (s *Store) UpsertApplication(ctx context.Context, q Querier, id, circleID, userID int64) error {
	_, err := q.Exec(ctx, `
		INSERT INTO cir_apply (id, circle_id, applicant_id, status)
		VALUES ($1,$2,$3,0)
		ON CONFLICT (circle_id, applicant_id) DO UPDATE
		SET status=0, reason=NULL, decided_by=NULL, decided_at=NULL, updated_at=now()
		WHERE cir_apply.status=2`, id, circleID, userID)
	return err
}

// ListApplications 圈主查看入圈申请（status<0 全部；否则按状态）。
func (s *Store) ListApplications(ctx context.Context, circleID int64, status int16) ([]Application, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, circle_id, applicant_id, status, COALESCE(reason,''), created_at
		FROM cir_apply WHERE circle_id=$1 AND ($2 < 0 OR status=$2)
		ORDER BY created_at DESC LIMIT 100`, circleID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Application, 0)
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.ID, &a.CircleID, &a.ApplicantID, &a.Status, &a.Reason, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DecideApplication 通过/拒绝待审申请（仅 status=0 生效）。
func (s *Store) DecideApplication(ctx context.Context, q Querier, circleID, applicantID, decidedBy int64, approve bool, reason string) (bool, error) {
	st := int16(2)
	if approve {
		st = 1
	}
	tag, err := q.Exec(ctx, `UPDATE cir_apply SET status=$4, reason=$5, decided_by=$3,
		decided_at=now(), updated_at=now()
		WHERE circle_id=$1 AND applicant_id=$2 AND status=0`, circleID, applicantID, decidedBy, st, nullStr(reason))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
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

// GetFeed 读取单条动态。
func (s *Store) GetFeed(ctx context.Context, id int64) (Feed, error) {
	var f Feed
	var images []byte
	err := s.pool.QueryRow(ctx, `SELECT id, circle_id, author_id, COALESCE(text,''), image_file_ids, is_announcement, created_at
		FROM cir_feed WHERE id=$1 AND status=0`, id).
		Scan(&f.ID, &f.CircleID, &f.AuthorID, &f.Text, &images, &f.IsAnnouncement, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, ErrNotFound
	}
	if err != nil {
		return f, err
	}
	_ = json.Unmarshal(images, &f.ImageFileIDs)
	return f, nil
}

// DeleteFeed 软删除动态并回写圈子动态数。
func (s *Store) DeleteFeed(ctx context.Context, q Querier, id, circleID int64) error {
	_, err := q.Exec(ctx, `UPDATE cir_feed SET status=1 WHERE id=$1 AND circle_id=$2 AND status=0`, id, circleID)
	if err != nil {
		return err
	}
	_, _ = q.Exec(ctx, `UPDATE cir_circle SET feed_count=GREATEST(feed_count-1,0) WHERE id=$1`, circleID)
	return nil
}

// ListFeeds 圈内动态/公告列表。
func (s *Store) ListFeeds(ctx context.Context, circleID int64, limit int) ([]Feed, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, circle_id, author_id, COALESCE(text,''), image_file_ids, is_announcement, created_at
		FROM cir_feed WHERE circle_id=$1 AND status=0
		ORDER BY is_announcement DESC, created_at DESC LIMIT $2`, circleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Feed, 0)
	for rows.Next() {
		var f Feed
		var images []byte
		if err := rows.Scan(&f.ID, &f.CircleID, &f.AuthorID, &f.Text, &images, &f.IsAnnouncement, &f.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(images, &f.ImageFileIDs)
		out = append(out, f)
	}
	return out, rows.Err()
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

// Mount 挂载行。
type Mount struct {
	ID             int64
	CircleID       int64
	ItemRef        string
	MountedBy      int64
	MountType      int16
	ApprovalStatus int16
	Status         int16
	CreatedAt      time.Time
}

// CreateMount 新建挂载（本人直接同意，他人待确认）。
func (s *Store) CreateMount(ctx context.Context, q Querier, id, circleID int64, itemRef string, mountedBy int64, mountType, approval int16) error {
	_, err := q.Exec(ctx, `
		INSERT INTO cir_mount (id, circle_id, item_ref, mounted_by, mount_type, approval_status, status)
		VALUES ($1,$2,$3,$4,$5,$6,0)`, id, circleID, itemRef, mountedBy, mountType, approval)
	return err
}

// CountMounts 单圈已同意挂载数。
func (s *Store) CountMounts(ctx context.Context, circleID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(1) FROM cir_mount WHERE circle_id=$1 AND status=0 AND approval_status=1`, circleID).Scan(&n)
	return n, err
}

// GetMount 取挂载。
func (s *Store) GetMount(ctx context.Context, id int64) (Mount, error) {
	var m Mount
	err := s.pool.QueryRow(ctx, `
		SELECT id, circle_id, item_ref, mounted_by, mount_type, approval_status, status, created_at
		FROM cir_mount WHERE id=$1`, id).
		Scan(&m.ID, &m.CircleID, &m.ItemRef, &m.MountedBy, &m.MountType, &m.ApprovalStatus, &m.Status, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mount{}, ErrNotFound
	}
	return m, err
}

// IsMounted 内容是否已挂载到该圈（status=0 且已同意）。
func (s *Store) IsMounted(ctx context.Context, circleID int64, itemRef string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM cir_mount
			WHERE circle_id=$1 AND item_ref=$2 AND status=0 AND approval_status=1)`, circleID, itemRef).Scan(&ok)
	return ok, err
}

// ListMounts 圈内挂载（status=0）。
func (s *Store) ListMounts(ctx context.Context, circleID int64) ([]Mount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, circle_id, item_ref, mounted_by, mount_type, approval_status, status, created_at
		FROM cir_mount WHERE circle_id=$1 AND status=0 ORDER BY created_at DESC`, circleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mount
	for rows.Next() {
		var m Mount
		if err := rows.Scan(&m.ID, &m.CircleID, &m.ItemRef, &m.MountedBy, &m.MountType, &m.ApprovalStatus, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PendingMount 待确认挂载 + 圈子名称。
type PendingMount struct {
	Mount
	CircleName string
}

// ListPendingMounts 待确认挂载（跨圈，供内容所有者处理）。
func (s *Store) ListPendingMounts(ctx context.Context, limit int) ([]PendingMount, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.circle_id, m.item_ref, m.mounted_by, m.mount_type, m.approval_status,
		       m.status, m.created_at, c.name
		FROM cir_mount m JOIN cir_circle c ON c.id=m.circle_id
		WHERE m.approval_status=0 AND m.status=0
		ORDER BY m.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingMount
	for rows.Next() {
		var p PendingMount
		if err := rows.Scan(&p.ID, &p.CircleID, &p.ItemRef, &p.MountedBy, &p.MountType,
			&p.ApprovalStatus, &p.Status, &p.CreatedAt, &p.CircleName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetMountApproval 挂载确认/拒绝。
func (s *Store) SetMountApproval(ctx context.Context, q Querier, id int64, approve bool) error {
	st := int16(2)
	if approve {
		st = 1
	}
	_, err := q.Exec(ctx, `UPDATE cir_mount SET approval_status=$2 WHERE id=$1`, id, st)
	return err
}

// MarkDemandTransferred 回写「已转入猫立方办」。
func (s *Store) MarkDemandTransferred(ctx context.Context, demandID int64, ref string) error {
	_, err := s.pool.Exec(ctx, `UPDATE cir_demand SET transfer_ref=$2 WHERE id=$1`, demandID, ref)
	return err
}
