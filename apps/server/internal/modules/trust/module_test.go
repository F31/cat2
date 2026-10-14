package trust

import (
	"testing"

	"catcube/internal/modules/trust/contract"
)

func TestHighRiskCategory(t *testing.T) {
	if got := highRiskCategory("本项目涉及金融投资与融资"); got != "finance" {
		t.Fatalf("金融类应命中: %q", got)
	}
	if got := highRiskCategory("医疗服务合作"); got != "medical" {
		t.Fatalf("医疗类应命中: %q", got)
	}
	if got := highRiskCategory("普通的产品介绍"); got != "" {
		t.Fatalf("普通文本不应命中: %q", got)
	}
}

func TestParseRef(t *testing.T) {
	typ, id := parseRef("content.project:123")
	if typ != "content.project" || id != 123 {
		t.Fatalf("parseRef 不符: %q %d", typ, id)
	}
	typ, id = parseRef("badref")
	if typ != "badref" || id != 0 {
		t.Fatalf("无冒号应原样返回: %q %d", typ, id)
	}
}

func TestValidReportReason(t *testing.T) {
	if !contract.ValidReportReason("fraud") {
		t.Fatal("fraud 应为合法举报原因")
	}
	if contract.ValidReportReason("nonsense") {
		t.Fatal("非法原因应拒绝")
	}
}
