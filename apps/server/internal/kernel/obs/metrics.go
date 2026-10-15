package obs

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

// Metrics 轻量指标注册表（Prometheus 文本格式导出；OTel 接入前的统一出口）。
// 计数用 Counter（只增），状态用 Gauge（可设）。标签以 `name{label="v"}` 形式作为键。
type Metrics struct {
	mu       sync.RWMutex
	counters map[string]float64
	gauges   map[string]float64
	alerts   map[string]int64
}

var defaultMetrics = NewMetrics()

// Default 全局指标表。
func Default() *Metrics { return defaultMetrics }

// NewMetrics 新建指标表。
func NewMetrics() *Metrics {
	return &Metrics{counters: map[string]float64{}, gauges: map[string]float64{}, alerts: map[string]int64{}}
}

// Inc 计数 +1（不存在则创建）。
func (m *Metrics) Inc(key string) { m.Add(key, 1) }

// Add 计数 +delta。
func (m *Metrics) Add(key string, delta float64) {
	m.mu.Lock()
	m.counters[key] += delta
	m.mu.Unlock()
}

// SetGauge 设置状态值。
func (m *Metrics) SetGauge(key string, v float64) {
	m.mu.Lock()
	m.gauges[key] = v
	m.mu.Unlock()
}

// Gauge 读取状态值。
func (m *Metrics) Gauge(key string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gauges[key]
}

// Counter 读取计数值。
func (m *Metrics) Counter(key string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.counters[key]
}

// Alert 告警（条件成立时记数并打点日志）。name 为告警名，msg 为详情。
func (m *Metrics) Alert(name, msg string) {
	m.mu.Lock()
	m.alerts[name]++
	m.mu.Unlock()
	Default().Inc(alertKey(name))
	slog.Warn("告警触发", "alert", name, "detail", msg)
}

// AlertCount 触发次数。
func (m *Metrics) AlertCount(name string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.alerts[name]
}

func alertKey(name string) string { return fmt.Sprintf("catcube_alerts_total{alert=%q}", name) }

// AlertsTotal 全部告警计数键（测试/看板用）。
func (m *Metrics) AlertsTotal() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int64, len(m.alerts))
	for k, v := range m.alerts {
		out[k] = v
	}
	return out
}

// Render 导出 Prometheus 文本格式。
func (m *Metrics) Render() string {
	m.mu.RLock()
	keys := make([]string, 0, len(m.counters)+len(m.gauges))
	for k := range m.counters {
		keys = append(keys, k)
	}
	for k := range m.gauges {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		name := strings.SplitN(k, "{", 2)[0]
		var v float64
		if cv, ok := m.counters[k]; ok {
			fmt.Fprintf(&b, "# TYPE %s counter\n", metricBase(name))
			v = cv
		} else {
			fmt.Fprintf(&b, "# TYPE %s gauge\n", metricBase(name))
			v = m.gauges[k]
		}
		fmt.Fprintf(&b, "%s %g\n", sanitizeMetric(k), v)
	}
	m.mu.RUnlock()
	return b.String()
}

func metricBase(name string) string { return strings.TrimSuffix(name, "_total") }

func sanitizeMetric(k string) string {
	// Prometheus 指标名/标签名必须为 [a-zA-Z_:][a-zA-Z0-9_:]*
	out := strings.ReplaceAll(k, "=\"", "=\"")
	return out
}
