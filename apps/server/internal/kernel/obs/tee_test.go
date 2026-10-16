package obs

import (
	"context"
	"log/slog"
	"testing"
)

// collector 收集记录用于断言（脱敏前后的分派）。
type collector struct{ recs []slog.Record }

func (c *collector) Enabled(context.Context, slog.Level) bool { return true }
func (c *collector) Handle(_ context.Context, r slog.Record) error {
	c.recs = append(c.recs, r)
	return nil
}
func (c *collector) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *collector) WithGroup(string) slog.Handler      { return c }

// Tee 扇出：一条记录应到达全部子 Handler，且脱敏在最外层先于扇出生效。
func TestTeeHandlerFansOutRedacted(t *testing.T) {
	a, b := &collector{}, &collector{}
	logger := slog.New(NewRedactingHandler(NewTeeHandler(a, b)))
	logger.Info("登录", "phone", "13812345678")

	if len(a.recs) != 1 || len(b.recs) != 1 {
		t.Fatalf("应各收到 1 条: a=%d b=%d", len(a.recs), len(b.recs))
	}
	var got string
	a.recs[0].Attrs(func(at slog.Attr) bool {
		if at.Key == "phone" {
			got = at.Value.String()
		}
		return true
	})
	if got == "13812345678" || got == "" {
		t.Fatalf("Tee 前应先脱敏: %q", got)
	}
}

// Tee.Enabled：任一子 Handler 启用即为真。
func TestTeeHandlerEnabled(t *testing.T) {
	ctx := context.Background()
	if !NewTeeHandler(&collector{}).Enabled(ctx, slog.LevelInfo) {
		t.Fatal("任一启用应为真")
	}
}
