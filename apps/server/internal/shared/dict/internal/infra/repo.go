// Package infra dict 仓储实现（PG，dct_ 前缀）。
package infra

import (
	"context"

	"catcube/internal/kernel/db"
	"catcube/internal/shared/dict/contract"
)

// Repo PG 仓储。
type Repo struct{ pool *db.Pool }

func NewRepo(pool *db.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) Options(ctx context.Context, dimension string) ([]contract.Option, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT dimension, code, label, COALESCE(parent_code,''), sort, status=1
		 FROM dct_taxonomy_option WHERE dimension=$1 ORDER BY sort, code`, dimension)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.Option
	for rows.Next() {
		var o contract.Option
		if err := rows.Scan(&o.Dimension, &o.Code, &o.Label, &o.ParentCode, &o.Sort, &o.Enabled); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *Repo) Resolve(ctx context.Context, code string) (contract.Option, error) {
	var o contract.Option
	err := r.pool.QueryRow(ctx,
		`SELECT dimension, code, label, COALESCE(parent_code,''), sort, status=1
		 FROM dct_taxonomy_option WHERE code=$1`, code).
		Scan(&o.Dimension, &o.Code, &o.Label, &o.ParentCode, &o.Sort, &o.Enabled)
	return o, err
}

func (r *Repo) SubjectTypes(ctx context.Context) ([]contract.SubjectType, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT code, name, COALESCE(icon_file_id,0), can_publish, home_visible, sort, status=1
		 FROM dct_subject_type ORDER BY sort, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.SubjectType
	for rows.Next() {
		var t contract.SubjectType
		if err := rows.Scan(&t.Code, &t.Name, &t.IconFileID, &t.CanPublish, &t.HomeVisible, &t.Sort, &t.Enabled); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
