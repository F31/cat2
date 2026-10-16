// Package llm LLM 端口适配器（一期先行接口；开发态实现用于联调与降级演练）。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"catcube/internal/ports"
)

// DevClient 开发态 LLM：按提示词关键词返回可预测的结构化 JSON；
// 输入含 FaultMarker 时返回错误（模拟上游不可用/超时），驱动降级路径（p6 §4.1）。
type DevClient struct{}

// FaultMarker 触发故障模拟。
const FaultMarker = "[FAULT]"

func NewDevClient() *DevClient { return &DevClient{} }

func (DevClient) Complete(_ context.Context, prompt string, _ map[string]any) ([]byte, error) {
	if strings.Contains(prompt, FaultMarker) {
		return nil, errors.New("llm: 上游不可用（dev 模拟）")
	}
	// 极简规则实现：把提示中的可见文本压缩为结构化字段，便于端到端联调。
	text := extractUserText(prompt)
	switch {
	case strings.Contains(prompt, "onboard"):
		return json.Marshal(map[string]any{
			"title": text,
			"fields": []map[string]string{
				{"field": "real_name", "value": text},
				{"field": "intro", "value": text},
			},
		})
	case strings.Contains(prompt, "search"):
		return json.Marshal(map[string]any{
			"keyword": text,
			"type":    "content.project",
			"region":  "310000",
		})
	default: // publish
		return json.Marshal(map[string]any{
			"title": text,
			"fields": []map[string]string{
				{"field": "intro", "value": text},
				{"field": "core_strength", "value": "待补充"},
			},
		})
	}
}

func (DevClient) Embed(_ context.Context, _ string) ([]float32, error) {
	// 一期不启用向量检索；返回定长零向量占位（接口可用）。
	return make([]float32, 8), nil
}

var _ ports.LLM = DevClient{}

func extractUserText(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "文本:"); ok {
			out := strings.TrimSpace(v)
			if len(out) > 50 {
				out = out[:50]
			}
			return out
		}
	}
	return "未识别"
}
