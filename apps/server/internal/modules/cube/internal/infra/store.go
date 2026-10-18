// Package infra cube 仓储（PG，cub_ 前缀）。
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

// Cube 立方行。
type Cube struct {
	ID           int64
	Name         string
	OrgID        int64
	LeaderID     int64
	HoneyID      int64
	RegionCode   string
	Industry     []string
	ChainStage   string
	Direction    string
	Resources    string
	Strength     string
	Cooperation  string
	PlanLevel    int16
	PlanExpireAt *time.Time
	Status       int16
	Version      int
	CreatedAt    time.Time
}

// Member 成员行。
type Member struct {
	ID       int64
	CubeID   int64
	UserID   int64
	Role     int16
	Status   int16
	JoinedAt time.Time
}

// Invite 邀请行。
type Invite struct {
	ID          int64
	CubeID      int64
	Code        string
	CreatedBy   int64
	ExpireAt    time.Time
	NeedConfirm bool
	Status      int16
}

// Store cube 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// 列清单（c 别名）。
const cubeCols = `c.id, c.name, COALESCE(c.owner_org_id,0), c.leader_id, c.honey_id, COALESCE(c.region_code,''),
	c.industry_codes, COALESCE(c.chain_stage,''), COALESCE(c.direction,''), COALESCE(c.resources,''),
	COALESCE(c.strength,''), COALESCE(c.cooperation,''), c.plan_level, c.plan_expire_at, c.status, c.version, c.created_at`

func scanCube(row pgx.Row) (Cube, error) {
	var c Cube
	var ind []byte
	err := row.Scan(&c.ID, &c.Name, &c.OrgID, &c.LeaderID, &c.HoneyID, &c.RegionCode,
		&ind, &c.ChainStage, &c.Direction, &c.Resources, &c.Strength, &c.Cooperation,
		&c.PlanLevel, &c.PlanExpireAt, &c.Status, &c.Version, &c.CreatedAt)
	if err != nil {
		return Cube{}, err
	}
	_ = json.Unmarshal(ind, &c.Industry)
	return c, nil
}

func scanCubes(rows pgx.Rows) ([]Cube, error) {
	var out []Cube
	for rows.Next() {
		c, err := scanCube(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateCube 新建立方。
func (s *Store) CreateCube(ctx context.Context, q Querier, c Cube) error {
	ind, _ := json.Marshal(c.Industry)
	_, err := q.Exec(ctx, `
		INSERT INTO cub_cube (id, name, owner_org_id, leader_id, honey_id, region_code, industry_codes,
			chain_stage, direction, resources, strength, cooperation, plan_level, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,0)`,
		c.ID, c.Name, nullID(c.OrgID), c.LeaderID, c.HoneyID, nullStr(c.RegionCode), ind,
		c.ChainStage, c.Direction, c.Resources, c.Strength, c.Cooperation, c.PlanLevel)
	return err
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

// GetCube 取立方。
func (s *Store) GetCube(ctx context.Context, id int64) (Cube, error) {
	c, err := scanCube(s.pool.QueryRow(ctx, `SELECT `+cubeCols+` FROM cub_cube c WHERE c.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Cube{}, ErrNotFound
	}
	return c, err
}

// ListCubesByUser 用户参与的立方（未移除、未解散）。
func (s *Store) ListCubesByUser(ctx context.Context, userID int64) ([]Cube, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+cubeCols+` FROM cub_cube c
		JOIN cub_member m ON m.cube_id=c.id
		WHERE m.user_ref=$1 AND m.status<>2 AND c.status<>2 ORDER BY c.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCubes(rows)
}

// AdminListCubes 后台立方列表（名称模糊，限制条数）。
func (s *Store) AdminListCubes(ctx context.Context, q string, limit int) ([]Cube, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if q == "" {
		return s.queryCubes(ctx, `SELECT `+cubeCols+` FROM cub_cube c ORDER BY c.created_at DESC LIMIT $1`, limit)
	}
	return s.queryCubes(ctx, `SELECT `+cubeCols+` FROM cub_cube c WHERE c.name ILIKE '%'||$1||'%' ORDER BY c.created_at DESC LIMIT $2`, q, limit)
}

func (s *Store) queryCubes(ctx context.Context, sql string, args ...any) ([]Cube, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCubes(rows)
}

// CountActiveCubes 用户参与的立方数（配额用）。
func (s *Store) CountActiveCubes(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(1) FROM cub_member m JOIN cub_cube c ON c.id=m.cube_id
		WHERE m.user_ref=$1 AND m.status<>2 AND c.status<>2`, userID).Scan(&n)
	return n, err
}

// AddMember 新增/复活成员（被移除者不可复活）。
func (s *Store) AddMember(ctx context.Context, q Querier, id, cubeID, userID int64, role int16, invitedBy int64) error {
	_, err := q.Exec(ctx, `
		INSERT INTO cub_member (id, cube_id, user_ref, role, invited_by, status, joined_at)
		VALUES ($1,$2,$3,$4,$5,0,now())
		ON CONFLICT (cube_id, user_ref) DO UPDATE SET role=EXCLUDED.role, status=0, joined_at=now()
		WHERE cub_member.status<>2`, id, cubeID, userID, role, nullID(invitedBy))
	return err
}

// GetMember 取成员（含已退出/被移除）。
func (s *Store) GetMember(ctx context.Context, cubeID, userID int64) (Member, bool, error) {
	var m Member
	err := s.pool.QueryRow(ctx,
		`SELECT id, cube_id, user_ref, role, status, joined_at FROM cub_member WHERE cube_id=$1 AND user_ref=$2`,
		cubeID, userID).Scan(&m.ID, &m.CubeID, &m.UserID, &m.Role, &m.Status, &m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, false, nil
	}
	if err != nil {
		return Member{}, false, err
	}
	return m, true, nil
}

// ListMembers 成员列表（不含被移除）。
func (s *Store) ListMembers(ctx context.Context, cubeID int64) ([]Member, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, cube_id, user_ref, role, status, joined_at FROM cub_member
		 WHERE cube_id=$1 AND status<>2 ORDER BY role, joined_at`, cubeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.CubeID, &m.UserID, &m.Role, &m.Status, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountMembers 有效成员数（不含被移除）。
func (s *Store) CountMembers(ctx context.Context, cubeID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(1) FROM cub_member WHERE cube_id=$1 AND status<>2`, cubeID).Scan(&n)
	return n, err
}

// SetMemberRole 变更成员角色。
func (s *Store) SetMemberRole(ctx context.Context, q Querier, cubeID, userID int64, role int16) error {
	_, err := q.Exec(ctx, `UPDATE cub_member SET role=$3 WHERE cube_id=$1 AND user_ref=$2 AND status<>2`, cubeID, userID, role)
	return err
}

// RemoveMember 移除成员（记录原因）。
func (s *Store) RemoveMember(ctx context.Context, q Querier, cubeID, userID int64, reason string) error {
	_, err := q.Exec(ctx, `UPDATE cub_member SET status=2, left_at=now(), remove_reason=$3
		WHERE cube_id=$1 AND user_ref=$2 AND status<>2`, cubeID, userID, reason)
	return err
}

// UpdateLeader 转让猫王（同步猫蜜与转让日志）。
func (s *Store) UpdateLeader(ctx context.Context, q Querier, cubeID, leaderID, honeyID int64, logJSON []byte) error {
	_, err := q.Exec(ctx, `UPDATE cub_cube SET leader_id=$2, honey_id=$3,
		transfer_log = transfer_log || $4::jsonb, version=version+1, updated_at=now() WHERE id=$1`,
		cubeID, leaderID, honeyID, logJSON)
	return err
}

// SetHoney 指定猫蜜（同步版本）。
func (s *Store) SetHoney(ctx context.Context, q Querier, cubeID, honeyID int64) error {
	_, err := q.Exec(ctx, `UPDATE cub_cube SET honey_id=$2, version=version+1, updated_at=now() WHERE id=$1`, cubeID, honeyID)
	return err
}

// SetCubeStatus 变更加立方状态。
func (s *Store) SetCubeStatus(ctx context.Context, q Querier, cubeID int64, status int16) error {
	_, err := q.Exec(ctx, `UPDATE cub_cube SET status=$2, version=version+1, updated_at=now() WHERE id=$1`, cubeID, status)
	return err
}

// SetPlan 更新订阅档位与到期（开通/续费）。
func (s *Store) SetPlan(ctx context.Context, cubeID int64, level int16, expireAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE cub_cube SET plan_level=$2, plan_expire_at=$3, version=version+1, updated_at=now() WHERE id=$1`,
		cubeID, level, expireAt)
	return err
}

// ListExpiredPlans 到期未回落的立方（SchedulerJob）。
func (s *Store) ListExpiredPlans(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM cub_cube
		WHERE plan_level>1 AND plan_expire_at IS NOT NULL AND plan_expire_at<now() AND status=0
		ORDER BY plan_expire_at LIMIT $1`, limit)
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

// InsertInvite 新建邀请码。
func (s *Store) InsertInvite(ctx context.Context, inv Invite) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO cub_invite (id, cube_id, code, created_by, expire_at, max_uses, need_confirm, status)
		VALUES ($1,$2,$3,$4,$5,NULL,$6,0)`,
		inv.ID, inv.CubeID, inv.Code, inv.CreatedBy, inv.ExpireAt, inv.NeedConfirm)
	return err
}

// GetInviteByCode 取邀请码。
func (s *Store) GetInviteByCode(ctx context.Context, code string) (Invite, error) {
	var inv Invite
	err := s.pool.QueryRow(ctx, `
		SELECT id, cube_id, code, created_by, expire_at, need_confirm, status
		FROM cub_invite WHERE code=$1`, code).
		Scan(&inv.ID, &inv.CubeID, &inv.Code, &inv.CreatedBy, &inv.ExpireAt, &inv.NeedConfirm, &inv.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	return inv, err
}

// IncrInviteUse 邀请码使用计数。
func (s *Store) IncrInviteUse(ctx context.Context, q Querier, code string) error {
	_, err := q.Exec(ctx, `UPDATE cub_invite SET used_count=used_count+1 WHERE code=$1`, code)
	return err
}
