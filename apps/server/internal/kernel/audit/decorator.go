package audit

import (
	"context"
	"net"
	"net/http"
	"strings"

	"catcube/internal/kernel/obs"
)

// actorOf / RequestIDOf 从上下文取操作者与 request_id（装配根注入 actor）。
func actorOf(ctx context.Context) int64 {
	if v, ok := ctx.Value(ctxKeyActor).(int64); ok {
		return v
	}
	return 0
}

// RequestIDOf 取 request_id（obs 中间件注入）。
func RequestIDOf(ctx context.Context) string { return obs.RequestID(ctx) }

// WithActor 注入操作者（认证中间件调用）。
func WithActor(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, ctxKeyActor, uid)
}

type ctxKey string

const ctxKeyActor ctxKey = "audit_actor"

// Point 审计点声明由 audit.go 定义；本文件提供「装饰器 + 入口中间件自动埋点」（p4 §5.1/§5.3）。

// HandlerFunc 标准 http 处理函数（避免 import 循环，用本地别名）。
type HandlerFunc = http.HandlerFunc

// Decorator 为 handler 注入审计：Action 必记；按需记失败/拒绝由 RecordFailed 控制。
// 用法：mux.Handle(path, rec.Decorate(point, actorType, channel, handler))
type Decorator struct {
	rec       *Recorder
	actorType ActorType
	channel   string
}

func NewDecorator(rec *Recorder, actorType ActorType, channel string) *Decorator {
	return &Decorator{rec: rec, actorType: actorType, channel: channel}
}

// Decorate 包装 handler：读取 X-Audit-Result 约定不可行，故对「显式拒绝」采取显式调用；
// 装饰器负责：注入 Entry 骨架（action/channel/actorType/request 上下文），并在响应为 4xx 时记 denied。
func (d *Decorator) Decorate(p Point, action string, next HandlerFunc) HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if d == nil || d.rec == nil {
			next(w, req)
			return
		}
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next(sw, req)
		if !p.LogFailed && sw.status < http.StatusBadRequest {
			return
		}
		res := ResultOK
		switch {
		case sw.status == http.StatusForbidden || sw.status == http.StatusUnauthorized:
			res = ResultDenied
		case sw.status >= http.StatusBadRequest:
			res = ResultFailed
		}
		_ = d.rec.Record(req.Context(), nil, Entry{
			ActorID: actorOf(req.Context()), ActorType: d.actorType, Channel: d.channel,
			Action: action, Result: res, IP: ClientIP(req), RequestID: RequestIDOf(req.Context()),
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// ClientIP 出口 IP（优先 X-Forwarded-For 首段）。
func ClientIP(req *http.Request) string {
	if v := req.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
