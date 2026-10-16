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

// DevOCR 开发态 OCR：返回可预测结构（真实供应商在接入时替换）。
type DevOCR struct{}

func NewDevOCR() *DevOCR { return &DevOCR{} }

func (DevOCR) Recognize(_ context.Context, req ports.OCRRequest) (ports.OCRResult, error) {
	if strings.Contains(req.ImageURL, "FAULT") {
		return ports.OCRResult{}, errors.New("ocr: 上游不可用（dev 模拟）")
	}
	return ports.OCRResult{Text: "样例识别文本", Fields: map[string]string{"org_name": "样例企业"}}, nil
}

// DevASR 开发态语音识别。
type DevASR struct{}

func NewDevASR() *DevASR { return &DevASR{} }

func (DevASR) Transcribe(_ context.Context, req ports.ASRRequest) (string, error) {
	if strings.Contains(req.VoiceURL, "FAULT") {
		return "", errors.New("asr: 上游不可用（dev 模拟）")
	}
	return "我是华东重工采购总监，寻找钣金供应商", nil
}

var (
	_ ports.OCR = DevOCR{}
	_ ports.ASR = DevASR{}
)
