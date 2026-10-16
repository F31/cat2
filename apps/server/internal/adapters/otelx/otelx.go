// Package otelx OpenTelemetry SDK 接入（p1 §5.6 / p6 §5.1）。
// 统一 Trace/Metric 出口：启用时经 OTLP gRPC 导出到 Collector；
// 未启用（或端点不可达）时保持全局 no-op，业务与既有日志/`/metrics` 不受影响。
package otelx

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Config OTel 接入配置（默认关闭，与特性开关一致：确认后再开）。
type Config struct {
	Enabled     bool
	ServiceName string  // service.name（默认 catcube）
	Endpoint    string  // OTLP gRPC 端点（默认 otel-collector:4317）
	Insecure    bool    // 明文 gRPC（集群内默认 true）
	SampleRatio float64 // 采样比例 0~1（默认 1；ParentBased 保证父采样优先）
}

func (c Config) withDefaults() Config {
	if c.ServiceName == "" {
		c.ServiceName = "catcube"
	}
	if c.Endpoint == "" {
		c.Endpoint = "otel-collector:4317"
	}
	if c.SampleRatio <= 0 || c.SampleRatio > 1 {
		c.SampleRatio = 1 // 未配置或越界 → 全采样
	}
	return c
}

// buildResource 统一的 resource（service.name + 环境/SDK 信息，Trace/Metric/Log 共用）。
func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", cfg.ServiceName)),
		resource.WithFromEnv(),      // OTEL_RESOURCE_ATTRIBUTES / OTEL_SERVICE_NAME
		resource.WithTelemetrySDK(), // 语言/SDK 版本
	)
	if err != nil {
		return nil, fmt.Errorf("otelx: 构建 resource 失败: %w", err)
	}
	return res, nil
}

// Setup 配置全局 TracerProvider/MeterProvider（OTLP gRPC）与 W3C 传播器。
// 无论是否启用都会设置 TraceContext+Baggage 传播器（供中间件提取上游 traceparent）。
// 返回的 shutdown 用于优雅退出（flush 未发送的 span/metric）。
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	cfg = cfg.withDefaults()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	noop := func(context.Context) error { return nil }
	if !cfg.Enabled {
		return noop, nil
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, err
	}

	topts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		topts = append(topts, otlptracegrpc.WithInsecure())
	}
	texp, err := otlptracegrpc.New(ctx, topts...)
	if err != nil {
		return nil, fmt.Errorf("otelx: 创建 trace exporter 失败: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(texp, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(tp)

	mopts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		mopts = append(mopts, otlpmetricgrpc.WithInsecure())
	}
	mexp, err := otlpmetricgrpc.New(ctx, mopts...)
	if err != nil {
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("otelx: 创建 metric exporter 失败: %w", err)
	}
	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(mexp, metric.WithInterval(15*time.Second))),
	)
	otel.SetMeterProvider(mp)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(e error) {
		_ = e // 静默：导出失败不得影响业务（如需可在此打点）
	}))

	return func(sctx context.Context) error {
		var firstErr error
		if err := tp.Shutdown(sctx); err != nil {
			firstErr = err
		}
		if err := mp.Shutdown(sctx); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	}, nil
}
