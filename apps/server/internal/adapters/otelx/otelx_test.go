package otelx

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// 关闭时：设置传播器、返回可调用的 no-op shutdown，不替换全局 provider。
func TestSetupDisabledNoop(t *testing.T) {
	before := otel.GetTracerProvider()
	shutdown, err := Setup(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("关闭时不应报错: %v", err)
	}
	if shutdown == nil {
		t.Fatal("应返回 shutdown 函数")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown 不应报错: %v", err)
	}
	if otel.GetTracerProvider() != before {
		t.Fatal("关闭时不应替换全局 TracerProvider")
	}
	// 传播器应已装配：可提取上游 traceparent
	const tid = "4bf92f3577b34da6a3ce929d0e0e4736"
	carrier := propagation.MapCarrier{"traceparent": "00-" + tid + "-00f067aa0ba902b7-01"}
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)
	if got := trace.SpanContextFromContext(ctx).TraceID().String(); got != tid {
		t.Fatalf("应能提取上游 trace：期望 %s，实际 %s", tid, got)
	}
}

// 启用时：构建成功（懒连接不阻塞）并替换全局 provider；shutdown 可调用而不 panic。
func TestSetupEnabledBuilds(t *testing.T) {
	before := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(before) })
	ctx := context.Background()
	shutdown, err := Setup(ctx, Config{
		Enabled: true, ServiceName: "catcube-test",
		Endpoint: "127.0.0.1:43999", Insecure: true, SampleRatio: 1,
	})
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	if otel.GetTracerProvider() == before {
		t.Fatal("启用时应替换全局 TracerProvider")
	}
	_, span := otel.Tracer("t").Start(ctx, "op")
	span.End()
	sctx, cancel := context.WithCancel(ctx)
	cancel()
	_ = shutdown(sctx) // 端点不可达：导出错误被静默，不得 panic
}

func TestConfigDefaults(t *testing.T) {
	c := Config{Enabled: true}.withDefaults()
	if c.ServiceName != "catcube" || c.Endpoint != "otel-collector:4317" || c.SampleRatio != 1 {
		t.Fatalf("默认值不符: %+v", c)
	}
	if (Config{SampleRatio: 2}).withDefaults().SampleRatio != 1 {
		t.Fatal("越界采样率应收敛为 1")
	}
}
