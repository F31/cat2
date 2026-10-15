// Package expiry 到期转态通用链路（p1 §6.5）：批量扫描 → 分批 → 幂等转移，供各模块复用。
//
// 复用方式：
//  1. 目标表带 `expires_at timestamptz` 与 `status smallint`，建部分索引：
//     CREATE INDEX ... ON <table>(expires_at) WHERE status = <生效状态>;
//  2. SchedulerJob 内调用 Runner.Run(ctx, spec)（每分钟/小时按需）；
//  3. 转移用单条 UPDATE（仅对仍处生效状态的行生效），天然幂等：重复执行不会二次转移。
package expiry

import (
	"context"
	"fmt"
	"regexp"

	"catcube/internal/kernel/db"
)

// Spec 到期转态规格。
type Spec struct {
	Table         string // 表名（不含 schema；白名单校验）
	ActiveStatus  int16  // 生效状态（部分索引谓词）
	ExpiredStatus int16  // 到期后状态
	BatchSize     int    // 每批条数（默认 200）
	MaxRounds     int    // 最大轮次（默认 50，防失控）
}

// OnBatch 每批转移后的回调（如发布到期事件）。
type OnBatch func(ctx context.Context, ids []int64) error

var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Run 批量到期转态，返回转移总数。
func Run(ctx context.Context, pool *db.Pool, spec Spec, onBatch OnBatch) (int, error) {
	if !identRe.MatchString(spec.Table) {
		return 0, fmt.Errorf("expiry: 非法表名 %q", spec.Table)
	}
	if spec.BatchSize <= 0 {
		spec.BatchSize = 200
	}
	if spec.MaxRounds <= 0 {
		spec.MaxRounds = 50
	}
	total := 0
	for round := 0; round < spec.MaxRounds; round++ {
		var ids []int64
		err := pool.InTx(ctx, func(tx db.Tx) error {
			rows, err := tx.Query(ctx, fmt.Sprintf(`
				UPDATE %s SET status=$2
				WHERE id IN (
					SELECT id FROM %s WHERE status=$1 AND expires_at IS NOT NULL AND expires_at < now()
					ORDER BY expires_at LIMIT $3
				)
				RETURNING id`, spec.Table, spec.Table),
				spec.ActiveStatus, spec.ExpiredStatus, spec.BatchSize)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		})
		if err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}
		total += len(ids)
		if onBatch != nil {
			if err := onBatch(ctx, ids); err != nil {
				return total, err
			}
		}
		if len(ids) < spec.BatchSize {
			return total, nil
		}
	}
	return total, nil
}
