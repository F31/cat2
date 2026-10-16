// OTel 指标桥接（p6 §5.1）：将内置指标注册表（Prom `/metrics` 的口径）
// 镜像为 OTel 异步 Gauge，统一带 `module` 标签，经 OTLP 周期导出。
// 未装配 MeterProvider 时为 no-op，`/metrics` 与业务不受影响。
package obs

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MeterName OTel 指标仪表名（镜像内置指标的 scope）。
const MeterName = "catcube/metrics"

// RegisterOTelMetrics 注册内置指标 → OTel 异步 Gauge 的桥接（幂等由调用方保证一次）。
func RegisterOTelMetrics() { _ = registerOTelMetrics(otel.GetMeterProvider().Meter(MeterName)) }

func registerOTelMetrics(meter metric.Meter) error {
	g, err := meter.Float64ObservableGauge("catcube.metric.snapshot",
		metric.WithDescription("内置指标快照（Prom /metrics 的 OTel 镜像，带 module 标签）"))
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for _, s := range Default().snapshot() {
			o.ObserveFloat64(g, s.value,
				metric.WithAttributes(attribute.String("module", s.module), attribute.String("metric", s.name)))
		}
		return nil
	}, g)
	return err
}

// modulePrefixes 指标名前缀 → 模块标签（长前缀优先，见注册表命名约定）。
var modulePrefixes = []struct{ prefix, module string }{
	{"catcube_ai_", "ai"},
	{"catcube_affiliation_", "profile"},
	{"catcube_audit_", "audit"},
	{"catcube_cert_", "verify"},
	{"catcube_contact_", "bff"},
	{"catcube_cube_", "cube"},
	{"catcube_events_", "eventbus"},
	{"catcube_export_", "export"},
	{"catcube_fulfillment_", "trade"},
	{"catcube_match_", "discovery"},
	{"catcube_member_", "membership"},
	{"catcube_order_", "trade"},
	{"catcube_outbox_", "eventbus"},
	{"catcube_pay_", "trade"},
	{"catcube_review_", "trust"},
	{"catcube_session_", "identity"},
	{"catcube_telemetry_", "telemetry"},
	{"catcube_user_", "identity"},
	{"catcube_alerts_", "obs"},
}

// metricModule 由指标名推导模块标签（未命中归 core）。
func metricModule(name string) string {
	for _, m := range modulePrefixes {
		if strings.HasPrefix(name, m.prefix) {
			return m.module
		}
	}
	return "core"
}
