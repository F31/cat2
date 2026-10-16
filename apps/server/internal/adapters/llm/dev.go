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
	case strings.Contains(prompt, "review"):
		return json.Marshal(map[string]any{
			"category": "fraud",
			"risk":     2,
			"summary":  "疑似虚假项目：" + text,
		})
	case strings.Contains(prompt, "search"):
		return json.Marshal(map[string]any{
			"keyword": text,
			"type":    "content.project",
			"region":  "310000",
		})
	case strings.Contains(prompt, "客服"):
		// 知识范围限平台规则/操作指引：命中平台主题给指引，否则转人工（p5 §5.2）。
		if inScope(text) {
			return json.Marshal(map[string]any{
				"answer":   "关于「" + text + "」：可在「我的」页按指引操作，如有疑问请联系人工客服。",
				"escalate": false,
			})
		}
		return json.Marshal(map[string]any{
			"answer":   "抱歉，该问题超出我的服务范围，已为你转人工客服。",
			"escalate": true,
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

// inScope 命中平台规则/操作主题才答；否则转人工（仅 dev 规则）。
func inScope(text string) bool {
	for _, kw := range []string{"会员", "认证", "实名", "发布", "订单", "退款", "名片", "搜索", "猫立方", "圈", "邀请", "联系方式"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

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
