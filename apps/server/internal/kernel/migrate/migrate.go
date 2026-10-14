// Package migrate 模块迁移执行器：每模块自己的迁移脚本，
// 按 DependsOn 拓扑顺序执行；记录执行历史。
package migrate

import (
	"context"
	"fmt"
	"sort"

	"catcube/internal/kernel/db"
)

// Migration 一条迁移。
type Migration struct {
	Version int    // 单调递增
	Name    string
	SQL     string
}

// Source 一个模块的迁移集合。
type Source struct {
	Module     string
	Migrations []Migration
}

// Runner 执行器。
type Runner struct {
	pool *db.Pool
}

func NewRunner(pool *db.Pool) *Runner { return &Runner{pool: pool} }

// Apply 按给定拓扑顺序执行未应用的迁移。
func (r *Runner) Apply(ctx context.Context, sources []Source) error {
	if _, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sys_migration (
			module   text NOT NULL,
			version  int  NOT NULL,
			name     text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (module, version)
		)`); err != nil {
		return err
	}
	for _, src := range sources {
		sort.Slice(src.Migrations, func(i, j int) bool { return src.Migrations[i].Version < src.Migrations[j].Version })
		for _, m := range src.Migrations {
			applied, err := r.isApplied(ctx, src.Module, m.Version)
			if err != nil {
				return err
			}
			if applied {
				continue
			}
			if err := r.pool.InTx(ctx, func(tx db.Tx) error {
				if _, err := tx.Exec(ctx, m.SQL); err != nil {
					return fmt.Errorf("migrate %s/%d %s: %w", src.Module, m.Version, m.Name, err)
				}
				_, err := tx.Exec(ctx,
					`INSERT INTO sys_migration (module, version, name) VALUES ($1,$2,$3)`,
					src.Module, m.Version, m.Name)
				return err
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Runner) isApplied(ctx context.Context, module string, version int) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(1) FROM sys_migration WHERE module=$1 AND version=$2`,
		module, version).Scan(&n)
	return n > 0, err
}
