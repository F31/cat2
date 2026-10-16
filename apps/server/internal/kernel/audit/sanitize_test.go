package audit

import "testing"

func TestSanitizeDetail(t *testing.T) {
	in := map[string]any{
		"phone":       "13812345678",
		"id_card_no":  "110101199003070019",
		"real_name":   "张三丰",
		"session_key": "s3cr3t",
		"amount_fen":  9900,
		"note":        "联系 13900000000 客户",
		"nested":      map[string]any{"contact_phone": "13700000000", "keep": "ok"},
		"list":        []any{"18800000000", "normal"},
	}
	out, _ := SanitizeDetail(in).(map[string]any)
	if out["session_key"] != "[redacted]" {
		t.Fatalf("密钥字段应整体屏蔽: %v", out["session_key"])
	}
	if out["phone"] == "13812345678" || out["id_card_no"] == "110101199003070019" {
		t.Fatalf("手机号/证件号应掩码: %v %v", out["phone"], out["id_card_no"])
	}
	if out["amount_fen"] != 9900 {
		t.Fatalf("普通字段应保留: %v", out["amount_fen"])
	}
	note, _ := out["note"].(string)
	if note == "联系 13900000000 客户" {
		t.Fatalf("值内手机号应抹除: %q", note)
	}
	nested, _ := out["nested"].(map[string]any)
	if nested["keep"] != "ok" || nested["contact_phone"] == "13700000000" {
		t.Fatalf("嵌套字段应脱敏并保留非敏感项: %v", nested)
	}
}
