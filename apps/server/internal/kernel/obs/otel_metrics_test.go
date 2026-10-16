package obs

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// 指标镜像：内置指标经 OTel 导出且统一带 module 标签。
func TestOTelMetricsBridgeModuleLabel(t *testing.T) {
	reader := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	const aiKey = "catcube_ai_call_total"
	const coreKey = "catcube_unknown_thing"
	Default().Add(aiKey, 3)
	Default().SetGauge(coreKey, 7)

	if err := registerOTelMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("采集失败: %v", err)
	}

	got := map[string]string{}
	for _, sm := range rm.ScopeMetrics {
		for _, mt := range sm.Metrics {
			g, ok := mt.Data.(metricdata.Gauge[float64])
			if !ok {
				continue
			}
			for _, dp := range g.DataPoints {
				name := attrString(dp.Attributes, "metric")
				got[name] = attrString(dp.Attributes, "module")
			}
		}
	}
	if got["catcube_ai_call"] != "ai" {
		t.Fatalf("catcube_ai_call 应带 module=ai: %v", got)
	}
	if got["catcube_unknown_thing"] != "core" {
		t.Fatalf("未命中前缀应归 core: %v", got)
	}
}

func attrString(set attribute.Set, key string) string {
	v, ok := set.Value(attribute.Key(key))
	if !ok {
		return ""
	}
	return v.AsString()
}
