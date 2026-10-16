package infra

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/db"
)

// Affiliation 归属记录。
type Affiliation struct {
	ID           int64
	UserID       int64
	OrgID        int64
	Role         string
	IsPrimary    bool
	Status       int16
	RejectReason string
	ProofIDs     []int64
}

// OrgAdmin 组织管理员记录。
type OrgAdmin struct {
	ID        int64
	OrgID     int64
	UserID    int64
	Status    int16
	InvitedBy int64
}

// InsertAffiliation 写归属（user+org 唯一：重复申请覆盖为待核验）。
func (s *Store) InsertAffiliation(ctx context.Context, tx db.Tx, a Affiliation) error {
	proof, _ := json.Marshal(a.ProofIDs)
	_, err := tx.Exec(ctx, `
		INSERT INTO vfy_affiliation (id, user_id, org_id, role, proof_file_ids, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,0,now(),now())
		ON CONFLICT (user_id, org_id) DO UPDATE
		SET role=EXCLUDED.role, proof_file_ids=EXCLUDED.proof_file_ids,
		    status=0, reject_reason=NULL, reviewer_id=NULL, reviewed_at=NULL, updated_at=now()`,
		a.ID, a.UserID, a.OrgID, a.Role, proof)
	return err
}

const affCols = `id, user_id, org_id, COALESCE(role,''), is_primary, status, COALESCE(reject_reason,''), COALESCE(proof_file_ids,'[]'::jsonb)`

func scanAff(row pgx.Row) (Affiliation, error) {
	var a Affiliation
	var proof []byte
	err := row.Scan(&a.ID, &a.UserID, &a.OrgID, &a.Role, &a.IsPrimary, &a.Status, &a.RejectReason, &proof)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	_ = json.Unmarshal(proof, &a.ProofIDs)
	return a, err
}

// AffiliationsOf 某账号的归属列表。
func (s *Store) AffiliationsOf(ctx context.Context, userID int64) ([]Affiliation, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+affCols+` FROM vfy_affiliation WHERE user_id=$1 ORDER BY is_primary DESC, created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Affiliation
	for rows.Next() {
		a, err := scanAff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAffiliation 单条。
func (s *Store) GetAffiliation(ctx context.Context, id int64) (Affiliation, error) {
	return scanAff(s.pool.QueryRow(ctx, `SELECT `+affCols+` FROM vfy_affiliation WHERE id=$1`, id))
}

// DecideAffiliation 核验/驳回/代处理（幂等：仅待核验可改）。
func (s *Store) DecideAffiliation(ctx context.Context, tx db.Tx, id, reviewerID int64, status int16, reason string) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE vfy_affiliation SET status=$3, reviewer_id=$2, reject_reason=$4, reviewed_at=now(), updated_at=now()
		WHERE id=$1 AND status=0`, id, reviewerID, status, nullStr(reason))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// SetPrimary 设为主单位（清除同用户其他主单位）。
func (s *Store) SetPrimary(ctx context.Context, tx db.Tx, userID, id int64) error {
	if _, err := tx.Exec(ctx, `UPDATE vfy_affiliation SET is_primary=false WHERE user_id=$1 AND is_primary`, userID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE vfy_affiliation SET is_primary=true, updated_at=now() WHERE id=$1 AND user_id=$2 AND status=1`, id, userID)
	return err
}

// OrphanAffiliations 超期未核验（平台代处理；sla 由调用方给定）。
func (s *Store) OrphanAffiliations(ctx context.Context, orgID int64, olderThanDays int, limit int) ([]Affiliation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+affCols+` FROM vfy_affiliation
		WHERE status=0 AND ($1=0 OR org_id=$1) AND created_at < now() - ($2 || ' days')::interval
		ORDER BY created_at LIMIT $3`, orgID, olderThanDays, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Affiliation
	for rows.Next() {
		a, err := scanAff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PendingAffiliations 组织待核验归属。
func (s *Store) PendingAffiliations(ctx context.Context, orgID int64, limit int) ([]Affiliation, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+affCols+` FROM vfy_affiliation WHERE org_id=$1 AND status=0 ORDER BY created_at LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Affiliation
	for rows.Next() {
		a, err := scanAff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// InsertOrgAdmin 邀请管理员（待确认）。
func (s *Store) InsertOrgAdmin(ctx context.Context, tx db.Tx, id, orgID, inviterID, inviteeID int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO vfy_org_admin (id, org_id, user_id, status, invited_by, created_at, updated_at)
		VALUES ($1,$2,$3,0,$4,now(),now())
		ON CONFLICT (org_id, user_id) DO UPDATE
		SET status=0, invited_by=EXCLUDED.invited_by, reason=NULL, confirmed_at=NULL, updated_at=now()`,
		id, orgID, inviteeID, inviterID)
	return err
}

// ConfirmOrgAdmin 被邀请人确认（仅待确认可确认）。
func (s *Store) ConfirmOrgAdmin(ctx context.Context, tx db.Tx, inviteID, userID int64) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE vfy_org_admin SET status=1, confirmed_at=now(), updated_at=now()
		WHERE id=$1 AND user_id=$2 AND status=0`, inviteID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// UpsertActiveOrgAdmin 直接置为在任（组织认证通过时首任/平台重置）。
func (s *Store) UpsertActiveOrgAdmin(ctx context.Context, tx db.Tx, id, orgID, userID int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO vfy_org_admin (id, org_id, user_id, status, confirmed_at, created_at, updated_at)
		VALUES ($1,$2,$3,1,now(),now(),now())
		ON CONFLICT (org_id, user_id) DO UPDATE
		SET status=1, reason=NULL, confirmed_at=now(), updated_at=now()`, id, orgID, userID)
	return err
}

// RemoveOrgAdmin 移除/停用管理员（平台重置）。
func (s *Store) RemoveOrgAdmin(ctx context.Context, tx db.Tx, orgID, userID int64, reason string) error {
	_, err := tx.Exec(ctx, `
		UPDATE vfy_org_admin SET status=2, reason=$3, updated_at=now()
		WHERE org_id=$1 AND user_id=$2 AND status IN (0,1)`, orgID, userID, reason)
	return err
}

// OrgAdminsOf 组织管理员列表。
func (s *Store) OrgAdminsOf(ctx context.Context, orgID int64) ([]OrgAdmin, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, user_id, status, COALESCE(invited_by,0) FROM vfy_org_admin
		WHERE org_id=$1 AND status IN (0,1) ORDER BY status, created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgAdmin
	for rows.Next() {
		var a OrgAdmin
		if err := rows.Scan(&a.ID, &a.OrgID, &a.UserID, &a.Status, &a.InvitedBy); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActiveOrgAdmin 是否在任。
func (s *Store) ActiveOrgAdmin(ctx context.Context, orgID, userID int64) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(1) FROM vfy_org_admin WHERE org_id=$1 AND user_id=$2 AND status=1`, orgID, userID).Scan(&n)
	return n > 0, err
}

var _ = pgx.ErrNoRows

// GetCert 单条认证。
func (s *Store) GetCert(ctx context.Context, id int64) (Cert, error) {
	return scanCertFull(s.pool.QueryRow(ctx, `SELECT `+certCols+`, reviewer_type FROM vfy_certification WHERE id=$1`, id))
}

// InsertPending 写入待审认证（含审核方与 extra）。
func (s *Store) InsertPending(ctx context.Context, tx db.Tx, c Cert, reviewerType int16, extra []byte) error {
	material, _ := json.Marshal(c.MaterialIDs)
	if len(extra) == 0 {
		extra = []byte("{}")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO vfy_certification (id, user_id, cert_type, target_org_id, material_file_ids, extra,
			status, reviewer_type, submitted_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now(),now(),now())`,
		c.ID, c.UserID, c.CertType, nullID(c.TargetOrgID), material, extra, c.Status, reviewerType)
	return err
}

// DecideCert 审核认证（幂等：仅待审可改）。
func (s *Store) DecideCert(ctx context.Context, tx db.Tx, id, reviewerID int64, status int16, reason string) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE vfy_certification SET status=$3, reviewer_id=$2, reject_reason=$4,
			reviewed_at=now(), updated_at=now()
		WHERE id=$1 AND status=0`, id, reviewerID, status, nullStr(reason))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// PendingCerts 待审列表（reviewerType=0 表示全部，orgID>0 时限定组织类）。
func (s *Store) PendingCerts(ctx context.Context, reviewerType int16, orgID int64, limit int) ([]Cert, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+certCols+`, reviewer_type FROM vfy_certification
		WHERE status=0 AND ($1=0 OR reviewer_type=$1) AND ($2=0 OR target_org_id=$2)
		ORDER BY submitted_at LIMIT $3`, reviewerType, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cert
	for rows.Next() {
		c, err := scanCertFull(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanCertFull(row pgx.Row) (Cert, error) {
	var c Cert
	var material []byte
	err := row.Scan(&c.ID, &c.UserID, &c.CertType, &c.TargetOrgID, &material,
		&c.RealNameEnc, &c.IDNoEnc, &c.Status, &c.RejectReason, &c.SubmitterAt, &c.ReviewedAt, &c.ReviewerType)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	_ = json.Unmarshal(material, &c.MaterialIDs)
	return c, err
}

// LatestOfType 某账号某类认证的最近一条。
func (s *Store) LatestOfType(ctx context.Context, userID int64, certType int16) (Cert, error) {
	return scanCertFull(s.pool.QueryRow(ctx, `
		SELECT `+certCols+`, reviewer_type FROM vfy_certification
		WHERE user_id=$1 AND cert_type=$2 ORDER BY submitted_at DESC LIMIT 1`, userID, certType))
}

// GetAffiliationByUserOrg 按 user+org 取归属。
func (s *Store) GetAffiliationByUserOrg(ctx context.Context, userID, orgID int64) (Affiliation, error) {
	return scanAff(s.pool.QueryRow(ctx, `SELECT `+affCols+` FROM vfy_affiliation WHERE user_id=$1 AND org_id=$2`, userID, orgID))
}
