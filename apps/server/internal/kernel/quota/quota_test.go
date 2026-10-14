package quota

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeCounter struct{ data map[string]int64 }

func newFakeCounter() *fakeCounter { return &fakeCounter{data: map[string]int64{}} }

func (c *fakeCounter) key(k string, s Scope, id int64, day time.Time) string {
	return k + "|" + string(s) + "|" + day.Format("2006-01-02") + "|" + itoa64(id)
}

func (c *fakeCounter) Incr(_ context.Context, k string, s Scope, id int64, day time.Time) (int64, error) {
	key := c.key(k, s, id, day)
	c.data[key]++
	return c.data[key], nil
}

func (c *fakeCounter) Get(_ context.Context, k string, s Scope, id int64, day time.Time) (int64, error) {
	return c.data[c.key(k, s, id, day)], nil
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func fakeLimits(vals map[string]int64) LimitSource {
	return func(_ context.Context, key string) (int64, error) {
		v, ok := vals[key]
		if !ok {
			return 0, errors.New("settings: 未配置 " + key)
		}
		return v, nil
	}
}

// 圈内发布：每人每日跨圈合计 3 条（与会员等级无关）。
func TestDailyLimit(t *testing.T) {
	eng := NewEngine(newFakeCounter(), fakeLimits(map[string]int64{"limit.circle.post.daily": 3}))
	eng.Register([]Def{{Key: "circle.post.daily", Window: WindowDay, Scope: ScopeUser,
		LimitKey: "limit.circle.post.daily", Reason: "每人每日跨圈合计最多 3 条"}})

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := eng.Consume(ctx, "circle.post.daily", 42); err != nil {
			t.Fatalf("第 %d 次不应超限: %v", i+1, err)
		}
	}
	err := eng.Consume(ctx, "circle.post.daily", 42)
	var ex *ExceededError
	if !errors.As(err, &ex) {
		t.Fatalf("第 4 次应超限: %v", err)
	}
	if ex.Reason == "" {
		t.Fatal("超限必须带 Reason 驱动前端提示")
	}
	// 另一用户不受影响
	if err := eng.Consume(ctx, "circle.post.daily", 43); err != nil {
		t.Fatal(err)
	}
}

func TestUnlimited(t *testing.T) {
	eng := NewEngine(newFakeCounter(), fakeLimits(map[string]int64{"limit.x": -1}))
	eng.Register([]Def{{Key: "x", Window: WindowTotal, Scope: ScopeUser, LimitKey: "limit.x"}})
	for i := 0; i < 100; i++ {
		if err := eng.Consume(context.Background(), "x", 1); err != nil {
			t.Fatal("负数=不限")
		}
	}
}

func TestUnregisteredKey(t *testing.T) {
	eng := NewEngine(newFakeCounter(), fakeLimits(nil))
	if err := eng.Check(context.Background(), "ghost", 1); err == nil {
		t.Fatal("未注册的限额键应报错")
	}
}
