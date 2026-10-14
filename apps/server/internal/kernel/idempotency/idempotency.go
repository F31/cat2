// Package idempotency 幂等键中间件与存储。
// 支付回调、创建订单、发布内容共用；创建/支付类接口必传 Idempotency-Key。
package idempotency

import (
	"context"
	"net/http"

	"catcube/pkg/errs"
)

const HeaderKey = "Idempotency-Key"

// Store 幂等记录存储（key → 已完成的响应）。
type Store interface {
	// Begin 占位：返回 false 表示已在处理/已处理。
	Begin(ctx context.Context, key string) (bool, error)
	// Complete 记录完成响应。
	Complete(ctx context.Context, key string, status int, body []byte) error
	// Get 已完成响应。
	Get(ctx context.Context, key string) (status int, body []byte, ok bool, err error)
}

// Middleware 强制创建/支付类接口携带幂等键并去重。
func Middleware(store Store) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(HeaderKey)
			if key == "" {
				writeErr(w, errs.BadRequest("000100", "缺少 Idempotency-Key"))
				return
			}
			if status, body, ok, err := store.Get(r.Context(), key); err == nil && ok {
				w.Header().Set("X-Idempotent-Replay", "true")
				w.WriteHeader(status)
				_, _ = w.Write(body)
				return
			}
			first, err := store.Begin(r.Context(), key)
			if err != nil {
				writeErr(w, errs.Internal("幂等存储错误"))
				return
			}
			if !first {
				writeErr(w, errs.Conflict("000101", "相同幂等键的请求正在处理中"))
				return
			}
			rw := &captureWriter{ResponseWriter: w, status: 200}
			next(rw, r)
			_ = store.Complete(r.Context(), key, rw.status, rw.body)
		}
	}
}

type captureWriter struct {
	http.ResponseWriter
	status int
	body   []byte
}

func (w *captureWriter) WriteHeader(s int) { w.status = s; w.ResponseWriter.WriteHeader(s) }
func (w *captureWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}

func writeErr(w http.ResponseWriter, e *errs.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(kindStatus(e.Kind))
	_, _ = w.Write([]byte(`{"code":"` + e.Code + `","message":"` + e.Msg + `"}`))
}

func kindStatus(k errs.Kind) int {
	switch k {
	case errs.KindBadRequest:
		return 400
	case errs.KindUnauthorized:
		return 401
	case errs.KindForbidden:
		return 403
	case errs.KindNotFound:
		return 404
	case errs.KindConflict:
		return 409
	case errs.KindRateLimited:
		return 429
	default:
		return 500
	}
}
