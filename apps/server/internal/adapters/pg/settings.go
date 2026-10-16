package pg

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/settings"
)

// Settings 业务配置存储（sys_settings，配置三层第三层：热生效）。
type Settings struct{ pool *db.Pool }

func NewSettings(pool *db.Pool) *Settings { return &Settings{pool: pool} }

func (s *Settings) Get(ctx context.Context, scope, key string) (settings.Entry, bool, error) {
	var e settings.Entry
	err := s.pool.QueryRow(ctx, `
		SELECT scope, key, value, version, COALESCE(operator,0)
		FROM sys_settings WHERE scope=$1 AND key=$2`, scope, key).
		Scan(&e.Scope, &e.Key, &e.Value, &e.Version, &e.Operator)
	if errors.Is(err, pgx.ErrNoRows) {
		return settings.Entry{}, false, nil
	}
	if err != nil {
		return settings.Entry{}, false, err
	}
	return e, true, nil
}

func (s *Settings) Set(ctx context.Context, e settings.Entry) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sys_settings (scope, key, value, type, version, operator, updated_at)
		VALUES ($1,$2,$3,'',1,$4,now())
		ON CONFLICT (scope, key) DO UPDATE
		SET value=EXCLUDED.value, operator=EXCLUDED.operator, version=sys_settings.version+1, updated_at=now()`,
		e.Scope, e.Key, e.Value, e.Operator)
	return err
}

func (s *Settings) List(ctx context.Context, scope string) ([]settings.Entry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT scope, key, value, version, COALESCE(operator,0)
		FROM sys_settings WHERE scope=$1 ORDER BY key`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []settings.Entry
	for rows.Next() {
		var e settings.Entry
		if err := rows.Scan(&e.Scope, &e.Key, &e.Value, &e.Version, &e.Operator); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

var _ settings.Store = (*Settings)(nil)
