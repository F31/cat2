//go:build integration

package app

import (
	"context"
	"encoding/json"
	"net/http"
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

// 5.4 一期审计点清单（详细设计 11.4）逐模块落地校验：注册表必须覆盖全部必记动作。
func TestIntegrationAuditChecklist(t *testing.T) {
	h := newTestApp(t)
	have := map[string]bool{}
	for _, p := range h.app.AuditPoints {
		have[p.Action] = true
		if p.Path == "" {
			t.Fatalf("审计点 %s 未关联路由", p.Action)
		}
	}
	required := []string{
		// identity / trust
		"identity:login:locked", "identity:user:deactivate", "identity:user:ban", "identity:user:unban",
		"trust:review:resolve", "trust:user:restrict",
		// trade / 导出
		"trade:refund:execute", "trade:fulfillment:retry", "trade:pay:notify", "export:create", "export:download",
		// membership
		"membership:sub:invite", "membership:sub:remove", "membership:master:transfer",
		"membership:member:adjust", "membership:price:set", "membership:enterprise:denied",
		// verify
		"verify:cert:review", "verify:affiliation:resolve", "verify:org_admin:reset",
		"verify:badge:config", "verify:badge:grant",
		// content / cube / circle
		"content:takedown",
		"cube:leader:transfer", "cube:member:remove", "cube:plan:change",
		"cube:dissolve", "cube:admin:ban", "cube:admin:dissolve",
		"circle:dissolve", "circle:admin:dissolve", "circle:convert",
		// channel
		"channel:operator:open", "channel:operator:suspend", "channel:settlement:review",
		"channel:settlement:reject", "channel:settlement:pay", "channel:attribution:correct",
		"channel:operator_fee:record", "channel:guarantee:refund", "channel:commission_rule:set",
		// profile / msg / notice / discovery / dict / admin
		"profile:contact:view", "profile:identity:modify:denied",
		"notice:announcement:broadcast", "discovery:weight:set",
		"ops:setting:set", "ops:feature_flag:set", "ops:dead_letter:replay", "ops:job:run",
		"ops:rbac:role:set", "ops:rbac:role:scope", "ops:rbac:role:create",
		"ops:rbac:role:delete", "ops:rbac:assign", "ai:prompt:save",
	}
	missing := []string{}
	for _, a := range required {
		if !have[a] {
			missing = append(missing, a)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("审计点清单缺失：%v", missing)
	}
}

// 5.4 运行期校验：跨模块配置动作按归属记录（佣金规则/匹配权重），并按 action 可检索。
func TestIntegrationAuditChecklistRuntime(t *testing.T) {
	h := newTestApp(t)
	_, admTok := loginAdmin(t, h)

	// 取当前匹配权重并原值回写（不改变业务行为，仅验证审计）
	_, env := h.do(http.MethodGet, "/api/admin/v1/settings", admTok, nil)
	var settingsBody struct {
		Items []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &settingsBody)
	old := ""
	for _, it := range settingsBody.Items {
		if it.Key == "dsc.weight.need" {
			old = it.Value
		}
	}
	if old == "" {
		t.Fatal("未找到 dsc.weight.need 配置")
	}
	if st, e := h.do(http.MethodPut, "/api/admin/v1/settings/dsc.weight.need", admTok, map[string]any{"value": old}); st != http.StatusOK || e.Code != 0 {
		t.Fatalf("回写配置失败: %d %v", st, e)
	}
	_, env = h.do(http.MethodGet, "/api/admin/v1/audits?action=discovery:weight:set&limit=10", admTok, nil)
	var auditsBody struct {
		Items []struct {
			Action string `json:"action"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &auditsBody)
	if len(auditsBody.Items) == 0 {
		t.Fatal("匹配权重变更未记录审计")
	}

	// 注销执行审计
	uid, tok := loginUser(t, h)
	if st, e := h.do(http.MethodPost, "/api/mp/v1/users/me/deactivate", tok, nil); st != http.StatusOK || e.Code != 0 {
		t.Fatalf("注销失败: %d %v", st, e)
	}
	found := false
	_, env = h.do(http.MethodGet, "/api/admin/v1/audits?action=identity:user:deactivate&limit=50", admTok, nil)
	_ = json.Unmarshal(env.Data, &auditsBody)
	for _, it := range auditsBody.Items {
		if it.Action == "identity:user:deactivate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("注销未记录审计（uid=%d）", uid)
	}
}
