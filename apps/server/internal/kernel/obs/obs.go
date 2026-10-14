// Package obs 可观测统一入口：结构化日志带 request_id/module/user_id(脱敏)。
// trace/metric 的 OTel 接入点收敛于此包（一期先落日志与指标标签约定）。
package obs

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"catcube/pkg/mask"
)

type ctxKey string

const (
	CtxRequestID ctxKey = "request_id"
	CtxModule    ctxKey = "module"
	CtxUserID    ctxKey = "user_id"
)

// WithRequestID 注入请求 ID。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, CtxRequestID, id)
}

// WithModule 注入模块标签。
func WithModule(ctx context.Context, m string) context.Context {
	return context.WithValue(ctx, CtxModule, m)
}

// Log 带上下文字段的结构化日志（user_id 以脱敏手机号场景为例展示脱敏约定）。
func Log(ctx context.Context) *slog.Logger {
	l := slog.Default()
	if v, ok := ctx.Value(CtxRequestID).(string); ok {
		l = l.With("request_id", v)
	}
	if v, ok := ctx.Value(CtxModule).(string); ok {
		l = l.With("module", v)
	}
	return l
}

// MaskPhone 日志脱敏出口（手机号一律经此输出）。
func MaskPhone(p string) string { return mask.Phone(p) }

// RequestIDMiddleware 为每个请求注入 request_id 并写访问日志。
func RequestIDMiddleware(next http.Handler) http.Handler {
	var seq int64
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seq++
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = time.Now().Format("20060102150405") + "-" + itoa(seq)
		}
		w.Header().Set("X-Request-ID", id)
		start := time.Now()
		next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), id)))
		slog.Info("http", "request_id", id, "method", r.Method, "path", r.URL.Path, "dur_ms", time.Since(start).Milliseconds())
	})
}

func itoa(n int64) string {
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
