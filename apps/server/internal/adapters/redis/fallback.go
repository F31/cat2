package redis

import (
	"context"
	"time"

	"catcube/internal/ports"
)

// Fallback 主备缓存：主（Redis）失败时读写回退到备（进程内），并回调 onErr 便于告警。
// 降级语义（p6 §4.4）：Redis 不可用不中断业务，仅退化为单机缓存。
type Fallback struct {
	Primary ports.Cache
	Backup  ports.Cache
	OnError func(error)
}

// NewFallback 构造主备缓存。
func NewFallback(primary, backup ports.Cache, onErr func(error)) *Fallback {
	return &Fallback{Primary: primary, Backup: backup, OnError: onErr}
}

func (f *Fallback) degraded(err error) {
	if err != nil && f.OnError != nil {
		f.OnError(err)
	}
}

func (f *Fallback) Get(ctx context.Context, key string) ([]byte, bool, error) {
	v, ok, err := f.Primary.Get(ctx, key)
	if err != nil {
		f.degraded(err)
		return f.Backup.Get(ctx, key)
	}
	return v, ok, nil
}

func (f *Fallback) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	// 双写：主恢复后立即可用，降级期间备也有值。
	if err := f.Primary.Set(ctx, key, val, ttl); err != nil {
		f.degraded(err)
	}
	return f.Backup.Set(ctx, key, val, ttl)
}

func (f *Fallback) Del(ctx context.Context, key string) error {
	if err := f.Primary.Del(ctx, key); err != nil {
		f.degraded(err)
	}
	if err := f.Backup.Del(ctx, key); err != nil {
		return err
	}
	return nil
}

var _ ports.Cache = (*Fallback)(nil)
