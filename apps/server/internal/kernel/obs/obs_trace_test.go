package obs

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

// 装配 OTel SDK 时：中间件记录 server span 并续接上游 traceparent（p6 §5.1）。
func TestRequestIDMiddlewareWithOTel(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exp),
		trace.WithSampler(trace.AlwaysSample()),
	)
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
		_ = tp.Shutdown(t.Context())
	}()

	const tid = "4bf92f3577b34da6a3ce929d0e0e4736"
	const psid = "00f067aa0ba902b7"
	srv := httptest.NewServer(RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/mp/v1/feed", nil)
	req.Header.Set("Traceparent", "00-"+tid+"-"+psid+"-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if got := resp.Header.Get("Traceparent"); !strings.HasPrefix(got, "00-"+tid+"-") {
		t.Fatalf("响应应续接上游 trace：%q", got)
	}
	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatal("应导出 server span")
	}
	s := spans[0]
	if got := s.SpanContext.TraceID().String(); got != tid {
		t.Fatalf("span trace_id 应续接上游：期望 %s，实际 %s", tid, got)
	}
	if s.Parent.SpanID().String() != psid {
		t.Fatalf("span 父 span_id 应为上游：期望 %s，实际 %s", psid, s.Parent.SpanID())
	}
}

// 未装配 SDK 时：回退轻量生成，trace 与响应头仍可关联（默认行为不变）。
func TestRequestIDMiddlewareWithoutOTel(t *testing.T) {
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(noop.NewTracerProvider())
	defer otel.SetTracerProvider(prevTP)
	const tid = "0af7651916cd43dd8448eb211c80319c"
	srv := httptest.NewServer(RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SDK 未启用时 trace 上下文仍从请求透传（供日志/outbox 关联）
		if TraceID(r.Context()) != tid {
			t.Errorf("ctx trace 应续接上游：%s", TraceID(r.Context()))
		}
	})))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/healthz", nil)
	req.Header.Set("Traceparent", "00-"+tid+"-00f067aa0ba902b7-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("Traceparent"); !strings.HasPrefix(got, "00-"+tid+"-") {
		t.Fatalf("未启用 SDK 时应沿用上游 trace：%q", got)
	}
}
