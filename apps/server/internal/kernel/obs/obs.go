// Package obs 可观测统一入口：结构化日志带 request_id/module/user_id(脱敏)。
// trace/metric 的 OTel 接入点收敛于此包（一期先落日志与指标标签约定）。
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"catcube/pkg/mask"
)

// TracerName 服务端 trace 仪表名（span 名/属性约定见 p6 §5.1）。
const TracerName = "catcube/http"

type ctxKey string

const (
	CtxRequestID ctxKey = "request_id"
	CtxModule    ctxKey = "module"
	CtxUserID    ctxKey = "user_id"
	CtxTraceID   ctxKey = "trace_id"
	CtxSpanID    ctxKey = "span_id"
)

// WithTrace 注入 trace 上下文（W3C Trace Context：trace_id 32 hex / span_id 16 hex）。
func WithTrace(ctx context.Context, traceID, spanID string) context.Context {
	return context.WithValue(context.WithValue(ctx, CtxTraceID, traceID), CtxSpanID, spanID)
}

// RequestID 取当前 request_id（无则空）。
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(CtxRequestID).(string); ok {
		return v
	}
	return ""
}

// TraceID 取当前 trace id（无则空）。
func TraceID(ctx context.Context) string {
	if v, ok := ctx.Value(CtxTraceID).(string); ok {
		return v
	}
	return ""
}

// NewSpanID 生成 16 hex span id。
func NewSpanID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

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
	if v, ok := ctx.Value(CtxUserID).(int64); ok {
		l = l.With("user_id", v)
	}
	if v, ok := ctx.Value(CtxTraceID).(string); ok && v != "" {
		l = l.With("trace_id", v)
	}
	if v, ok := ctx.Value(CtxSpanID).(string); ok && v != "" {
		l = l.With("span_id", v)
	}
	return l
}

// MaskPhone 日志脱敏出口（手机号一律经此输出）。
func MaskPhone(p string) string { return mask.Phone(p) }

// RequestIDMiddleware 为每个请求注入 request_id 与 trace 上下文（W3C traceparent 透传/生成）并写访问日志。
// 已装配 OTel SDK 时记录 server span 并导出；未启用时回退轻量生成（日志/响应头仍可关联）。
func RequestIDMiddleware(next http.Handler) http.Handler {
	var seq int64
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seq++
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = time.Now().Format("20060102150405") + "-" + itoa(seq)
		}
		w.Header().Set("X-Request-ID", id)

		// 提取上游 W3C trace context，起 server span（no-op 时不产生 span）
		pctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		pctx, span := otel.Tracer(TracerName).Start(pctx, r.Method+" "+r.URL.Path,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("url.path", r.URL.Path)),
		)
		var traceID, spanID string
		if sc := span.SpanContext(); sc.IsValid() {
			traceID, spanID = sc.TraceID().String(), sc.SpanID().String()
		} else {
			// SDK 未启用：沿用上游 trace_id（若有），否则新建；span_id 每次新建
			traceID, _ = parseTraceparent(r.Header.Get("Traceparent"))
			if traceID == "" {
				traceID = randomHex(16) // 32 hex
			}
			spanID = NewSpanID()
		}
		w.Header().Set("Traceparent", "00-"+traceID+"-"+spanID+"-01")

		ctx := WithTrace(WithRequestID(pctx, id), traceID, spanID)
		start := time.Now()
		next.ServeHTTP(w, r.WithContext(ctx))
		dur := time.Since(start)
		span.SetAttributes(attribute.Int64("http.server.duration_ms", dur.Milliseconds()))
		span.End()
		slog.InfoContext(ctx, "http", "request_id", id, "trace_id", traceID, "span_id", spanID,
			"method", r.Method, "path", r.URL.Path, "dur_ms", dur.Milliseconds())
	})
}

// SpanFromContext 取当前 span（供业务补录属性；未装配 SDK 时返回 no-op span）。
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// parseTraceparent 解析 W3C traceparent，返回 trace_id 与 parent span_id（非法返回空）。
func parseTraceparent(tp string) (traceID, spanID string) {
	parts := strings.Split(tp, "-")
	if len(parts) != 4 || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return "", ""
	}
	for _, ch := range parts[1] + parts[2] {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return "", ""
		}
	}
	return parts[1], parts[2]
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
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

const (
	CtxSessionKey ctxKey = "session_key"
)

// WithUserID 注入当前用户（日志关联；session_key/手机号等不入日志）。
func WithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, CtxUserID, uid)
}

// secretKeys 值必须整体屏蔽的字段名（小写比较）。
var secretKeys = map[string]bool{
	"session_key": true, "sessionkey": true, "access_token": true, "refresh_token": true,
	"token": true, "password": true, "secret": true, "appsecret": true,
	"encrypted_data": true, "iv": true, "enc_key": true, "private_key": true,
	"id_card_no": true, "id_no": true, "real_name": true, "phone": true, "contact_phone": true,
}

// redactValue 值级脱敏：屏蔽密钥类；手机号/证件号形态做掩码。
func redactValue(key, val string) string {
	if secretKeys[strings.ToLower(key)] {
		if key == "phone" || key == "contact_phone" {
			return mask.Phone(val)
		}
		if key == "id_card_no" || key == "id_no" {
			return mask.IDCard(val)
		}
		if key == "real_name" {
			return mask.Name(val)
		}
		return "[redacted]"
	}
	// 值本身像手机号/证件号 → 掩码；字符串内嵌的手机号/证件号一并抹除
	if isPhoneLike(val) {
		return mask.Phone(val)
	}
	if isIDLike(val) {
		return mask.IDCard(val)
	}
	return scrubEmbedded(val)
}

var (
	phoneRe = regexp.MustCompile(`(?:^|\D)(1[3-9]\d{9})(?:\D|$)`)
	idRe    = regexp.MustCompile(`(?:^|\D)(\d{17}[\dXx])(?:\D|$)`)
)

// scrubEmbedded 抹除字符串中内嵌的手机号/证件号。
func scrubEmbedded(s string) string {
	for i := 0; i < 3; i++ { // 迭代至稳定（相邻匹配）
		before := s
		s = phoneRe.ReplaceAllStringFunc(s, func(m string) string {
			return replaceDigits(m, mask.Phone)
		})
		s = idRe.ReplaceAllStringFunc(s, func(m string) string {
			return replaceDigits(m, mask.IDCard)
		})
		if s == before {
			break
		}
	}
	return s
}

// replaceDigits 对匹配中的数字片段做掩码，保留非数字包裹字符。
func replaceDigits(m string, f func(string) string) string {
	start, end := 0, len(m)
	for start < end && (m[start] < '0' || m[start] > '9') {
		start++
	}
	for end > start && (m[end-1] < '0' || m[end-1] > '9') {
		end--
	}
	if start >= end {
		return m
	}
	return m[:start] + f(m[start:end]) + m[end:]
}

func isPhoneLike(s string) bool {
	if len(s) != 11 || s[0] != '1' {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isIDLike(s string) bool {
	if len(s) < 15 || len(s) > 18 {
		return false
	}
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		} else if r != 'X' && r != 'x' {
			return false
		}
	}
	return digits >= 15
}

// RedactingHandler 包裹底层 Handler：出日志前统一脱敏（防误记敏感信息）。
type RedactingHandler struct{ Inner slog.Handler }

// NewRedactingHandler 包装。
func NewRedactingHandler(inner slog.Handler) slog.Handler { return RedactingHandler{Inner: inner} }

// TeeHandler 将同一条记录扇出到多个 Handler（控制台 + OTel 日志桥接，p6 §5.1）。
// Enabled 取任一为真；Handle/WithAttrs/WithGroup 同步作用于全部子 Handler。
func NewTeeHandler(handlers ...slog.Handler) slog.Handler { return TeeHandler{Handlers: handlers} }

// TeeHandler 多路日志 Handler。
type TeeHandler struct{ Handlers []slog.Handler }

func (h TeeHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, x := range h.Handlers {
		if x.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (h TeeHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, x := range h.Handlers {
		if !x.Enabled(ctx, r.Level) {
			continue
		}
		if err := x.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(h.Handlers))
	for i, x := range h.Handlers {
		out[i] = x.WithAttrs(attrs)
	}
	return TeeHandler{Handlers: out}
}

func (h TeeHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(h.Handlers))
	for i, x := range h.Handlers {
		out[i] = x.WithGroup(name)
	}
	return TeeHandler{Handlers: out}
}

func (h RedactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.Inner.Enabled(ctx, l)
}

func (h RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(redactAttr(a))
		return true
	})
	return h.Inner.Handle(ctx, nr)
}

func (h RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redactAttr(a)
	}
	return RedactingHandler{Inner: h.Inner.WithAttrs(out)}
}

func (h RedactingHandler) WithGroup(name string) slog.Handler {
	return RedactingHandler{Inner: h.Inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindGroup:
		g := a.Value.Group()
		out := make([]slog.Attr, len(g))
		for i, x := range g {
			out[i] = redactAttr(x)
		}
		a.Value = slog.GroupValue(out...)
	case slog.KindString:
		a.Value = slog.StringValue(redactValue(a.Key, a.Value.String()))
	}
	return a
}
