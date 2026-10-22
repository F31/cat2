// Package infra verify 仓储（PG，vfy_ 前缀）。
package infra

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/db"
	"catcube/internal/modules/verify/contract"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 记录不存在")

// Cert 认证记录。
type Cert struct {
	ID           int64
	UserID       int64
	CertType     int16
	TargetOrgID  int64
	MaterialIDs  []int64
	Extra        map[string]string
	RealNameEnc  []byte
	IDNoEnc      []byte
	Status       int16
	RejectReason string
	ReviewerType int16
	SubmitterAt  time.Time
	ReviewedAt   *time.Time
}

// Store verify 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// Insert 写认证记录（调用方提供 id，便于与事件同事务）。
func (s *Store) Insert(ctx context.Context, tx db.Tx, c Cert) error {
	material, _ := json.Marshal(c.MaterialIDs)
	_, err := tx.Exec(ctx, `
		INSERT INTO vfy_certification (id, user_id, cert_type, target_org_id, material_file_ids,
			real_name_enc, id_no_enc, status, reject_reason, reviewer_type, submitted_at, reviewed_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,1,now(),now(),now(),now())`,
		c.ID, c.UserID, c.CertType, nullID(c.TargetOrgID), material,
		c.RealNameEnc, c.IDNoEnc, c.Status, nullStr(c.RejectReason))
	return err
}

const certCols = `id, user_id, cert_type, COALESCE(target_org_id,0), COALESCE(material_file_ids,'[]'::jsonb),
	COALESCE(extra,'{}'::jsonb), real_name_enc, id_no_enc, status, COALESCE(reject_reason,''), submitted_at, reviewed_at`

func scanCert(row pgx.Row) (Cert, error) {
	var c Cert
	var material, extra []byte
	err := row.Scan(&c.ID, &c.UserID, &c.CertType, &c.TargetOrgID, &material, &extra,
		&c.RealNameEnc, &c.IDNoEnc, &c.Status, &c.RejectReason, &c.SubmitterAt, &c.ReviewedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if len(material) > 0 {
		_ = json.Unmarshal(material, &c.MaterialIDs)
	}
	if len(extra) > 0 {
		_ = json.Unmarshal(extra, &c.Extra)
	}
	return c, nil
}

// LatestRealname 最近一条实名认证记录。
func (s *Store) LatestRealname(ctx context.Context, userID int64) (Cert, error) {
	return scanCert(s.pool.QueryRow(ctx,
		`SELECT `+certCols+` FROM vfy_certification
		 WHERE user_id=$1 AND cert_type=1 ORDER BY submitted_at DESC, id DESC LIMIT 1`, userID))
}

// LatestApprovedRealnames 批量查询多个账号最近一条已通过实名记录（用于后台列表展示）。
func (s *Store) LatestApprovedRealnames(ctx context.Context, userIDs []int64) ([]Cert, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (user_id) `+certCols+`
		  FROM vfy_certification
		 WHERE cert_type=1 AND status=1 AND user_id = ANY($1)
		 ORDER BY user_id, submitted_at DESC, id DESC`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cert
	for rows.Next() {
		c, err := scanCert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CertsOf 某账号全部认证记录（新→旧）。
func (s *Store) CertsOf(ctx context.Context, userID int64) ([]Cert, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+certCols+` FROM vfy_certification WHERE user_id=$1 ORDER BY submitted_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cert
	for rows.Next() {
		c, err := scanCert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GrantBadge 授予标识（幂等：已撤销则复活）。
func (s *Store) GrantBadge(ctx context.Context, tx db.Tx, subjectType string, subjectID int64, code string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO vfy_badge_grant (subject_type, subject_id, badge_code)
		VALUES ($1,$2,$3)
		ON CONFLICT (subject_type, subject_id, badge_code) DO UPDATE SET revoked_at=NULL`,
		subjectType, subjectID, code)
	return err
}

// BadgesOf 有效标识（关联定义取 label/level）。
func (s *Store) BadgesOf(ctx context.Context, subjectType string, subjectID int64) ([]contract.Badge, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.code, d.label, d.level
		FROM vfy_badge_grant g JOIN vfy_badge_def d ON d.code=g.badge_code
		WHERE g.subject_type=$1 AND g.subject_id=$2 AND g.revoked_at IS NULL
		ORDER BY d.level DESC, d.code`, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.Badge
	for rows.Next() {
		var b contract.Badge
		if err := rows.Scan(&b.Code, &b.Label, &b.Level); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func nullID(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
