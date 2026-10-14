// Package retry 指数退避重试。
package retry

import (
	"context"
	"time"
)

// Do 以指数退避重试 fn，最多 attempts 次；base 为初始间隔。
func Do(ctx context.Context, attempts int, base time.Duration, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(base << i):
		}
	}
	return err
}
