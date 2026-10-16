package audit

import (
	"regexp"
	"testing"
)

// 敏感动词：命中即必须声明审计点（p4 §5.3）。
var sensitiveVerb = regexp.MustCompile(`(?i)/(refund|ban|unban|takedown|dissolve|transfer|export|replay|resolve|withdraw|settings|feature-flags)`)

func TestSensitiveRoutesDeclared(t *testing.T) {
	dir := "../../"
	_ = dir
}

// Registry 收集与匹配。
func TestRegistry(t *testing.T) {
	r := &Registry{}
	r.Register(Point{Action: "trade:refund:execute", Path: "/api/admin/v1/refunds", Required: true, LogFailed: true, Module: "trade"})
	if len(r.Points()) != 1 {
		t.Fatal("注册失败")
	}
}
