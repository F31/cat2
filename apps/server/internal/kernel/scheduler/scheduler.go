// Package scheduler 定时任务：PG 咨询锁保证全局单活；批量、幂等、可重入。
// 用途：到期转态（会员/订阅/临时圈/需求/分摊截止/订单超时/邀请链接）、日结、对账。
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"catcube/internal/kernel/db"
)

// Job 定时任务。
type Job struct {
	Name     string
	Interval time.Duration
	Run      func(ctx context.Context) error
}

// Runner 执行器（scheduler 角色）。lockKey 为咨询锁键。
type Runner struct {
	pool    *db.Pool
	lockKey int64
	jobs    []Job
}

func NewRunner(pool *db.Pool, lockKey int64) *Runner {
	return &Runner{pool: pool, lockKey: lockKey}
}

func (r *Runner) Register(jobs []Job) { r.jobs = append(r.jobs, jobs...) }

// Start 启动循环；单活：拿不到咨询锁的实例空转待命。
func (r *Runner) Start(ctx context.Context) {
	locked, err := r.pool.AdvisoryLock(ctx, r.lockKey)
	if err != nil {
		slog.Error("scheduler 咨询锁失败", "err", err)
		return
	}
	if !locked {
		slog.Info("scheduler 待命（另一实例持锁）")
		<-ctx.Done()
		return
	}
	defer r.pool.AdvisoryUnlock(context.Background(), r.lockKey)

	var wg sync.WaitGroup
	for _, j := range r.jobs {
		wg.Add(1)
		go func(j Job) {
			defer wg.Done()
			ticker := time.NewTicker(j.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := j.Run(ctx); err != nil {
						slog.Error("scheduler 任务失败", "job", j.Name, "err", err)
					}
				}
			}
		}(j)
	}
	wg.Wait()
}
