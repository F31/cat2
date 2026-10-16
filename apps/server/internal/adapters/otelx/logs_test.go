package otelx

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// capturingExporter 内存捕获导出的日志记录（同步 SimpleProcessor）。
type capturingExporter struct {
	mu   sync.Mutex
	recs []sdklog.Record
}

func (e *capturingExporter) Export(_ context.Context, rs []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range rs {
		e.recs = append(e.recs, rs[i].Clone())
	}
	return nil
}

func (e *capturingExporter) Shutdown(context.Context) error   { return nil }
func (e *capturingExporter) ForceFlush(context.Context) error { return nil }

func (e *capturingExporter) snapshot() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.recs...)
}

// slog 记录应转为 OTel 日志记录：级别/正文/属性（含组展平）。
func TestLogHandlerBridgesToOTel(t *testing.T) {
	exp := &capturingExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.Background()) })

	logger := slog.New(NewLogHandler(lp, "catcube-test"))
	logger.With("module", "ai").WithGroup("req").
		WarnContext(context.Background(), "上游降级", "status", 503, "ok", false)

	recs := exp.snapshot()
	if len(recs) != 1 {
		t.Fatalf("应导出 1 条，实际 %d", len(recs))
	}
	r := recs[0]
	if got := r.Body().AsString(); got != "上游降级" {
		t.Fatalf("正文不符: %q", got)
	}
	if r.Severity() != otellog.SeverityWarn1 || r.SeverityText() != "WARN" {
		t.Fatalf("级别不符: %v/%s", r.Severity(), r.SeverityText())
	}
	attrs := map[string]string{}
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		attrs[string(kv.Key)] = kv.Value.Emit()
		return true
	})
	if attrs["module"] != "ai" {
		t.Fatalf("应保留 module 属性: %v", attrs)
	}
	if attrs["req.status"] != "503" || attrs["req.ok"] != "false" {
		t.Fatalf("组属性应展平为点分键: %v", attrs)
	}
}

// 关闭时 SetupLogs 返回 nil handler（保持原控制台出口）。
func TestSetupLogsDisabled(t *testing.T) {
	h, shutdown, err := SetupLogs(context.Background(), Config{Enabled: false})
	if err != nil || h != nil {
		t.Fatalf("关闭时应为 nil handler: h=%v err=%v", h, err)
	}
	if shutdown == nil {
		t.Fatal("应返回可调用的 no-op shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown 不应报错: %v", err)
	}
}
