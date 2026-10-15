package obs

import (
	"log/slog"
	"strings"
	"testing"
)

func TestMetricsRenderAndAlert(t *testing.T) {
	m := NewMetrics()
	slog.SetDefault(slog.New(NewRedactingHandler(slog.NewTextHandler(&strings.Builder{}, nil))))
	m.Inc("catcube_events_relayed_total")
	m.Add("catcube_events_relayed_total", 4)
	m.SetGauge("catcube_outbox_backlog", 7)
	if got := m.Counter("catcube_events_relayed_total"); got != 5 {
		t.Fatalf("counter=%v", got)
	}
	if got := m.Gauge("catcube_outbox_backlog"); got != 7 {
		t.Fatalf("gauge=%v", got)
	}
	m.Alert("outbox_backlog_high", "积压 7")
	if m.AlertCount("outbox_backlog_high") != 1 {
		t.Fatal("告警计数应为 1")
	}
	out := m.Render()
	if !strings.Contains(out, "catcube_outbox_backlog 7") || !strings.Contains(out, "catcube_events_relayed_total 5") {
		t.Fatalf("导出缺少指标: %s", out)
	}
	if !strings.Contains(out, "# TYPE catcube_outbox_backlog gauge") {
		t.Fatalf("应含 TYPE 行: %s", out)
	}
}
