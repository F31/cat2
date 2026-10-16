package ai

import (
	"strings"
	"testing"

	"catcube/internal/modules/ai/contract"
)

// 隐私红线（p5 §1.7）：手机号/证件号/邮箱/微信号在构造提示词前剥离。
func TestSanitizeForPrompt(t *testing.T) {
	in := "我叫张三，手机 13812345678，证件 110101199003070019，邮箱 a.b@corp.com，微信：abc_12345，找钣金供应商"
	out, removed := sanitizeForPrompt(in)
	for _, leak := range []string{"13812345678", "110101199003070019", "a.b@corp.com", "abc_12345"} {
		if strings.Contains(out, leak) {
			t.Fatalf("提示词泄漏敏感信息 %q: %s", leak, out)
		}
	}
	if !strings.Contains(out, "找钣金供应商") {
		t.Fatalf("业务文本应保留: %s", out)
	}
	if removed < 4 {
		t.Fatalf("应识别 4 类敏感信息，实际 %d", removed)
	}
}

func TestBuildPromptNoSensitive(t *testing.T) {
	p, removed := buildPrompt("内容草稿（publish）", "project", "联系人 13900001111")
	if strings.Contains(p, "13900001111") || removed == 0 {
		t.Fatalf("统一入口应过滤: %q removed=%d", p, removed)
	}
}

func TestDropUnknownFields(t *testing.T) {
	fields := []struct{ f, v string }{{"intro", "x"}, {"hacked", "y"}, {"", "z"}}
	in := make([]contract.Field, 0, len(fields))
	for _, f := range fields {
		in = append(in, contract.Field{Field: f.f, Value: f.v})
	}
	allowed := map[string]bool{"intro": true, "core_strength": true}
	out, dropped := dropUnknownFields(in, allowed)
	if dropped != 2 || len(out) != 1 || out[0].Field != "intro" {
		t.Fatalf("字段白名单收敛失败: out=%+v dropped=%d", out, dropped)
	}
	if _, d := dropUnknownFields(in, nil); d != 0 {
		t.Fatal("无字段池时不应丢弃")
	}
}

func TestDefaultPromptAndFilterCodes(t *testing.T) {
	if defaultPrompt("onboard") == "" || defaultPrompt("publish") == "" || defaultPrompt("search") == "" {
		t.Fatal("默认提示词应齐备")
	}
}
