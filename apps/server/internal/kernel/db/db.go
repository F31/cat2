// Package db PostgreSQL 连接与事务管理（pgx）。
// 事务管理器保证业务表 + outbox 同事务写入。
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tx 事务句柄（pgx.Tx）。
type Tx = pgx.Tx

// Pool 连接池封装。
type Pool struct {
	*pgxpool.Pool
}

// Connect 建立连接池。
func Connect(ctx context.Context, url string) (*Pool, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db: 连接失败: %w", err)
	}
	if err := p.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db: ping 失败: %w", err)
	}
	return &Pool{p}, nil
}

// InTx 在事务中执行 fn；fn 返回 error 则回滚。
// 业务表与 outbox 事件在同一 tx 内写入，保证「业务已改但事件丢失」不存在。
func (p *Pool) InTx(ctx context.Context, fn func(tx Tx) error) error {
	tx, err := p.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // 已提交时 Rollback 为 no-op
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AdvisoryLock 尝试获取 PG 咨询锁（scheduler 全局单活）。
func (p *Pool) AdvisoryLock(ctx context.Context, key int64) (bool, error) {
	var ok bool
	err := p.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&ok)
	return ok, err
}

// AdvisoryUnlock 释放咨询锁。
func (p *Pool) AdvisoryUnlock(ctx context.Context, key int64) error {
	_, err := p.Exec(ctx, `SELECT pg_advisory_unlock($1)`, key)
	return err
}
