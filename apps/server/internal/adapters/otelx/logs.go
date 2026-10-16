// OTel 日志桥接（p6 §5.1）：slog 记录经此处转 OTel Logs SDK 经 OTLP 导出，
// 与控制台 JSON 出口并存（Tee）；未启用时不装配，日志仍走原出口。

package otelx

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// SetupLogs 配置 OTel Logs SDK（OTLP gRPC）并返回对应的 slog.Handler。
// 未启用时返回 (nil, no-op, nil)：调用方保持原控制台出口不变。
func SetupLogs(ctx context.Context, cfg Config) (slog.Handler, func(context.Context) error, error) {
	cfg = cfg.withDefaults()
	noop := func(context.Context) error { return nil }
	if !cfg.Enabled {
		return nil, noop, nil
	}
	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	lopts := []otlploggrpc.Option{otlploggrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		lopts = append(lopts, otlploggrpc.WithInsecure())
	}
	lexp, err := otlploggrpc.New(ctx, lopts...)
	if err != nil {
		return nil, nil, fmt.Errorf("otelx: 创建 log exporter 失败: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(lexp, sdklog.WithExportInterval(2*time.Second))),
	)
	return NewLogHandler(lp, cfg.ServiceName), lp.Shutdown, nil
}

// NewLogHandler slog→OTel 日志桥接：属性经 OTel 记录导出；
// 使用 InfoContext/Log(ctx) 时由 ctx 关联 trace_id/span_id。
func NewLogHandler(lp *sdklog.LoggerProvider, scope string) slog.Handler {
	return &logHandler{logger: lp.Logger(scope)}
}

type logHandler struct {
	logger otellog.Logger
	kvs    []attribute.KeyValue
	prefix string
}

func (h *logHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	var rec otellog.Record
	rec.SetTimestamp(r.Time)
	rec.SetObservedTimestamp(time.Now())
	rec.SetSeverity(toSeverity(r.Level))
	rec.SetSeverityText(r.Level.String())
	rec.SetBody(attribute.StringValue(r.Message))
	rec.AddAttributes(h.kvs...)
	r.Attrs(func(a slog.Attr) bool {
		rec.AddAttributes(attrToKVs(h.prefix, a)...)
		return true
	})
	h.logger.Emit(ctx, rec)
	return nil
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	kvs := make([]attribute.KeyValue, 0, len(h.kvs)+len(attrs))
	kvs = append(kvs, h.kvs...)
	for _, a := range attrs {
		kvs = append(kvs, attrToKVs(h.prefix, a)...)
	}
	return &logHandler{logger: h.logger, kvs: kvs, prefix: h.prefix}
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	prefix := name
	if h.prefix != "" {
		prefix = h.prefix + "." + name
	}
	return &logHandler{logger: h.logger, kvs: h.kvs, prefix: prefix}
}

func toSeverity(l slog.Level) otellog.Severity {
	switch {
	case l < slog.LevelInfo:
		return otellog.SeverityDebug1
	case l < slog.LevelWarn:
		return otellog.SeverityInfo1
	case l < slog.LevelError:
		return otellog.SeverityWarn1
	default:
		return otellog.SeverityError1
	}
}

// attrToKVs 将 slog.Attr 转为 OTel 属性（组属性展平为点分键）。
func attrToKVs(prefix string, a slog.Attr) []attribute.KeyValue {
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		var out []attribute.KeyValue
		for _, ga := range v.Group() {
			out = append(out, attrToKVs(key, ga)...)
		}
		return out
	}
	if key == "" {
		return nil
	}
	return []attribute.KeyValue{{Key: attribute.Key(key), Value: toOTelValue(v)}}
}

func toOTelValue(v slog.Value) attribute.Value {
	switch v.Kind() {
	case slog.KindString:
		return attribute.StringValue(v.String())
	case slog.KindInt64:
		return attribute.Int64Value(v.Int64())
	case slog.KindUint64:
		return attribute.Int64Value(int64(v.Uint64()))
	case slog.KindFloat64:
		return attribute.Float64Value(v.Float64())
	case slog.KindBool:
		return attribute.BoolValue(v.Bool())
	case slog.KindDuration:
		return attribute.Int64Value(v.Duration().Milliseconds())
	case slog.KindTime:
		return attribute.StringValue(v.Time().Format(time.RFC3339Nano))
	default:
		return attribute.StringValue(fmt.Sprint(v.Any()))
	}
}
