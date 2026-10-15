// Package infra discovery 仓储（PG，dsc_ 前缀；投影只读模型）。
package infra

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/db"
	"catcube/internal/modules/discovery/internal/domain"
)

// refsOf 用户个人/组织两个文档引用（避免 SQL 内 `'user:'||$1` 参数类型歧义）。
func refsOf(userID int64) (string, string) {
	s := strconv.FormatInt(userID, 10)
	return "user:" + s, "org:" + s
}

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 记录不存在")

// Doc 搜索文档（投影）。
type Doc struct {
	Ref         string
	Type        string
	SubjectID   int64
	Title       string
	Cover       string
	Region      string
	Industry    []string
	Need        []string
	Offer       []string
	MemberLevel int16
	Verified    bool
	Active      bool
	Version     int
	LastActive  time.Time
}

// Candidate 匹配候选（含回填展示信息）。
type Candidate struct {
	Ref         string
	Score       float64
	Reason      []domain.ReasonItem
	Rank        int
	Type        string
	Title       string
	Cover       string
	Region      string
	MemberLevel int16
	Verified    bool
}

// Filter 搜索过滤条件。
type Filter struct {
	Keyword  string
	Type     string
	Region   string
	Industry []string
	Need     []string
	Offer    []string
	Limit    int
}

// Store discovery 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

const docCols = `doc_ref, doc_type, subject_id, title, COALESCE(cover,''), COALESCE(region_code,''),
	industry, need, offer, member_level, verified, active, search_doc_version, last_active_at`

func scanDoc(row pgx.Row) (Doc, error) {
	var d Doc
	err := row.Scan(&d.Ref, &d.Type, &d.SubjectID, &d.Title, &d.Cover, &d.Region,
		&d.Industry, &d.Need, &d.Offer, &d.MemberLevel, &d.Verified, &d.Active, &d.Version, &d.LastActive)
	return d, err
}

// UpsertDoc 写入/更新投影（版本防旧事件覆盖：仅当版本不低于现存才生效）。
func (s *Store) UpsertDoc(ctx context.Context, d Doc) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO dsc_search_doc
			(doc_ref, doc_type, subject_id, title, cover, region_code, industry, need, offer,
			 member_level, verified, active, search_doc_version, last_active_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now(),now())
		ON CONFLICT (doc_ref) DO UPDATE SET
			doc_type=EXCLUDED.doc_type, subject_id=EXCLUDED.subject_id, title=EXCLUDED.title,
			cover=EXCLUDED.cover, region_code=EXCLUDED.region_code, industry=EXCLUDED.industry,
			need=EXCLUDED.need, offer=EXCLUDED.offer, active=EXCLUDED.active,
			search_doc_version=EXCLUDED.search_doc_version, last_active_at=now(), updated_at=now()
		WHERE EXCLUDED.search_doc_version >= dsc_search_doc.search_doc_version`,
		d.Ref, d.Type, d.SubjectID, d.Title, d.Cover, d.Region,
		arr(d.Industry), arr(d.Need), arr(d.Offer), d.MemberLevel, d.Verified, d.Active, d.Version)
	return tag.RowsAffected() > 0, err
}

// SetActive 置灰/恢复（下架、关闭）。
func (s *Store) SetActive(ctx context.Context, ref string, active bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE dsc_search_doc SET active=$2, updated_at=now() WHERE doc_ref=$1`, ref, active)
	return err
}

// SetVerified 更新实名/认证标识（user 与 org 文档）。
func (s *Store) SetVerified(ctx context.Context, userID int64) error {
	u, o := refsOf(userID)
	_, err := s.pool.Exec(ctx,
		`UPDATE dsc_search_doc SET verified=true, updated_at=now() WHERE doc_ref IN ($1,$2)`, u, o)
	return err
}

// SetMemberLevel 更新会员等级（同分排序依据）。
func (s *Store) SetMemberLevel(ctx context.Context, userID int64, level int16) error {
	u, o := refsOf(userID)
	_, err := s.pool.Exec(ctx,
		`UPDATE dsc_search_doc SET member_level=$3, updated_at=now() WHERE doc_ref IN ($1,$2)`, u, o, level)
	return err
}

// TouchActive 刷新活跃时间。
func (s *Store) TouchActive(ctx context.Context, userID int64) error {
	u, o := refsOf(userID)
	_, err := s.pool.Exec(ctx,
		`UPDATE dsc_search_doc SET last_active_at=now() WHERE doc_ref IN ($1,$2)`, u, o)
	return err
}

// GetDoc 取文档。
func (s *Store) GetDoc(ctx context.Context, ref string) (Doc, error) {
	d, err := scanDoc(s.pool.QueryRow(ctx, `SELECT `+docCols+` FROM dsc_search_doc WHERE doc_ref=$1`, ref))
	if errors.Is(err, pgx.ErrNoRows) {
		return Doc{}, ErrNotFound
	}
	return d, err
}

// CandidatePool 召回候选：供给∩我需求 / 需求∩我供给 / 行业 / 区域。
func (s *Store) CandidatePool(ctx context.Context, my domain.Profile, excludeRef string, limit int) ([]Doc, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+docCols+` FROM dsc_search_doc
		 WHERE active=true AND doc_ref<>$1
		   AND (offer && $2 OR need && $3 OR industry && $4 OR (region_code<>'' AND region_code=$5))
		 ORDER BY last_active_at DESC
		 LIMIT $6`,
		excludeRef, arr(my.Need), arr(my.Offer), arr(my.Industry), my.Region, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Doc
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AllProfiles 全量人/组织画像文档（候选周期刷新用）。
func (s *Store) AllProfiles(ctx context.Context, limit int) ([]Doc, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+docCols+` FROM dsc_search_doc WHERE active=true AND doc_type IN ('user','org') ORDER BY last_active_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Doc
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AllActiveDocs 全量活跃文档（重建/回放用）。
func (s *Store) AllActiveDocs(ctx context.Context, limit int) ([]Doc, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+docCols+` FROM dsc_search_doc WHERE active=true ORDER BY last_active_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Doc
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ReplaceCandidates 覆盖某用户的候选（预计算写模型）。
func (s *Store) ReplaceCandidates(ctx context.Context, userID int64, list []domain.Ranked) error {
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM dsc_match_candidates WHERE user_id=$1`, userID); err != nil {
			return err
		}
		for i, c := range list {
			reason, _ := json.Marshal(c.Reason)
			if _, err := tx.Exec(ctx, `
				INSERT INTO dsc_match_candidates (user_id, candidate_ref, score, reason, rank, computed_at)
				VALUES ($1,$2,$3,$4,$5,now())
				ON CONFLICT (user_id, candidate_ref) DO UPDATE
				SET score=EXCLUDED.score, reason=EXCLUDED.reason, rank=EXCLUDED.rank, computed_at=now()`,
				userID, c.Ref, c.Score, reason, i+1); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListCandidates 读预计算候选（首页只读此表；offset 用于换一批）。
func (s *Store) ListCandidates(ctx context.Context, userID int64, limit, offset int) ([]Candidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.candidate_ref, c.score, c.reason, c.rank,
		       d.doc_type, d.title, COALESCE(d.cover,''), COALESCE(d.region_code,''), d.member_level, d.verified
		  FROM dsc_match_candidates c
		  JOIN dsc_search_doc d ON d.doc_ref=c.candidate_ref
		 WHERE c.user_id=$1 AND d.active=true
		 ORDER BY c.rank ASC, c.score DESC
		 LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		var c Candidate
		var reason []byte
		if err := rows.Scan(&c.Ref, &c.Score, &reason, &c.Rank, &c.Type, &c.Title, &c.Cover, &c.Region, &c.MemberLevel, &c.Verified); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(reason, &c.Reason)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Search 关键词 + 标签检索（pg_trgm；标签与关键词独立可用）。
func (s *Store) Search(ctx context.Context, f Filter) ([]Doc, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+docCols+` FROM dsc_search_doc
		 WHERE active=true
		   AND ($1='' OR doc_type=$1)
		   AND ($2='' OR title % $2)
		   AND ($3='' OR region_code=$3)
		   AND (cardinality($4::text[])=0 OR industry && $4)
		   AND (cardinality($5::text[])=0 OR need && $5)
		   AND (cardinality($6::text[])=0 OR offer && $6)
		 ORDER BY (CASE WHEN $2<>'' THEN similarity(title,$2) ELSE 0 END) DESC, last_active_at DESC
		 LIMIT $7`,
		f.Type, f.Keyword, f.Region, arr(f.Industry), arr(f.Need), arr(f.Offer), f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Doc
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteUser 注销：删除该用户的文档与候选。
func (s *Store) DeleteUser(ctx context.Context, userID int64) error {
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM dsc_match_candidates WHERE user_id=$1`, userID); err != nil {
			return err
		}
		u, o := refsOf(userID)
		_, err := tx.Exec(ctx, `DELETE FROM dsc_search_doc WHERE doc_ref IN ($1,$2)`, u, o)
		return err
	})
}

// Reset 清空投影（回放重建前调用）。
func (s *Store) Reset(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM dsc_match_candidates`); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM dsc_search_doc`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO dsc_projection_meta(name, last_rebuilt_at) VALUES('search', now())
		ON CONFLICT (name) DO UPDATE SET last_rebuilt_at=now()`)
	return err
}

func arr(a []string) []string {
	if a == nil {
		return []string{}
	}
	return a
}
