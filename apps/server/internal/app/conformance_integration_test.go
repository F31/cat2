//go:build integration

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"catcube/internal/kernel/module"
	"catcube/internal/kernel/statemachine"
)

// 9.2 全部状态机全表（从装配结果读取，避免跨模块 internal 导入限制）。
func TestIntegrationStateMachinesFullTable(t *testing.T) {
	h := newTestApp(t)
	defs := h.app.StateMachines
	if len(defs) < 10 {
		t.Fatalf("状态机数量过少: %d", len(defs))
	}
	names := map[string]bool{}
	eng := statemachine.NewEngine(nil)
	for _, def := range defs {
		if err := statemachine.Validate(def); err != nil {
			t.Fatalf("状态机 %s 定义非法: %v", def.Name, err)
		}
		if names[def.Name] {
			t.Fatalf("状态机重名: %s", def.Name)
		}
		names[def.Name] = true

		states := map[string]bool{}
		for _, s := range def.States {
			states[s] = true
		}
		outbound := map[string]bool{}
		for _, tr := range def.Transitions {
			if !states[tr.From] || !states[tr.To] {
				t.Fatalf("状态机 %s 转移引用未声明状态: %s->%s", def.Name, tr.From, tr.To)
			}
			if tr.Event == "" {
				t.Fatalf("状态机 %s 存在空事件转移", def.Name)
			}
			outbound[tr.From] = true
		}
		terminal := map[string]bool{}
		for _, s := range def.Terminal {
			terminal[s] = true
		}
		for _, s := range def.States {
			if !terminal[s] && !outbound[s] {
				t.Fatalf("状态机 %s 非终态 %s 无出边", def.Name, s)
			}
		}
		if _, err := eng.Apply(context.Background(), def, 1, def.States[0], "__illegal_event__", "*"); err == nil {
			t.Fatalf("状态机 %s 非法事件未被拒绝", def.Name)
		}
	}
	for _, need := range []string{
		"verify.realname", "verify.cert", "verify.affiliation",
		"channel.operator", "channel.settlement",
	} {
		if !names[need] {
			t.Fatalf("缺少关键状态机 %s", need)
		}
	}
}

// 9.4 审计覆盖面：敏感后台路由必须声明审计点。
func TestIntegrationAuditCoverage(t *testing.T) {
	h := newTestApp(t)
	routes := map[string]string{} // path -> method
	points := map[string]bool{}
	for _, m := range h.app.Modules {
		reg := module.NewRegistry(m.Name())
		if err := m.Register(reg); err != nil {
			t.Fatalf("模块 %s 重新注册失败: %v", m.Name(), err)
		}
		for _, rt := range reg.Routes {
			routes[rt.Path] = rt.Method
		}
		for _, p := range reg.AuditPoints {
			points[p.Path] = true
		}
	}
	// 资金/治理类敏感动词（后台路由必须留审计）
	sensitive := []string{
		"/ban", "/unban", "/restrict", "/takedown", "/resolve", "/refund", "/retry",
		"/transfer", "/adjust", "/broadcast", "/dissolve", "/suspend", "/open",
		"/review", "/reject", "/pay", "/correct", "/fees", "/guarantee", "/replay",
	}
	isSensitive := func(p string) bool {
		for _, v := range sensitive {
			if strings.Contains(p, v) {
				return true
			}
		}
		return false
	}
	var gaps []string
	for p, method := range routes {
		// 只校验变更类（GET/HEAD 为读取，不要求审计）
		if method == "GET" || method == "HEAD" || !strings.HasPrefix(p, "/api/admin/") || !isSensitive(p) {
			continue
		}
		if !points[p] {
			gaps = append(gaps, p)
		}
	}
	if len(gaps) > 0 {
		t.Fatalf("敏感后台路由缺审计点（%d）：%v", len(gaps), gaps)
	}
}

// 9.5 契约路径：后台模块路由均需在 admin.yaml 声明；channel/verify 事件 Schema 存在。
func TestIntegrationAdminOpenAPICoversRoutes(t *testing.T) {
	h := newTestApp(t)
	raw, err := os.ReadFile(filepath.Join(contractsDir(t, "openapi"), "admin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	var missing []string
	for _, m := range h.app.Modules {
		reg := module.NewRegistry(m.Name())
		if err := m.Register(reg); err != nil {
			t.Fatalf("模块 %s 重新注册失败: %v", m.Name(), err)
		}
		for _, rt := range reg.Routes {
			if !rt.Admin {
				continue
			}
			rel := strings.TrimPrefix(rt.Path, "/api/admin/v1")
			if !strings.Contains(text, "\n  "+rel+":") {
				missing = append(missing, rt.Path)
			}
		}
	}
	if len(missing) > 0 {
		t.Fatalf("admin.yaml 缺少后台路由（%d）：%v", len(missing), missing)
	}
	// channel / verify 事件 Schema
	events := map[string]bool{}
	files, err := os.ReadDir(filepath.Join(contractsDir(t, "events")))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		events[strings.TrimSuffix(f.Name(), ".json")] = true
	}
	for _, e := range []string{
		"channel.OperatorStatusChanged.v1", "channel.CommissionAccrued.v1", "channel.SettlementPaid.v1",
		"verify.CertificationApproved.v1", "verify.CertificationRejected.v1", "verify.AffiliationVerified.v1", "verify.OrgAdminChanged.v1",
	} {
		if !events[e] {
			t.Fatalf("缺少事件 Schema: %s", e)
		}
	}
}
