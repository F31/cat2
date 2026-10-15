package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoSucceedsAfterRetries(t *testing.T) {
	n := 0
	err := Do(context.Background(), 3, time.Millisecond, func() error {
		n++
		if n < 3 {
			return errors.New("retry")
		}
		return nil
	})
	if err != nil || n != 3 {
		t.Fatalf("应在第 3 次成功: n=%d err=%v", n, err)
	}
}

func TestDoExhaustsAttempts(t *testing.T) {
	sentinel := errors.New("always")
	n := 0
	err := Do(context.Background(), 2, time.Millisecond, func() error { n++; return sentinel })
	if !errors.Is(err, sentinel) || n != 2 {
		t.Fatalf("应重试耗尽并返回最后一次错误: n=%d err=%v", n, err)
	}
}

func TestDoStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n := 0
	err := Do(ctx, 5, time.Second, func() error { n++; return errors.New("x") })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回 ctx 取消: %v", err)
	}
	if n != 1 {
		t.Fatalf("取消后不应继续重试: n=%d", n)
	}
}
