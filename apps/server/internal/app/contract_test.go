package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	circlecontract "catcube/internal/modules/circle/contract"
	contentcontract "catcube/internal/modules/content/contract"
	cubecontract "catcube/internal/modules/cube/contract"
	idcontract "catcube/internal/modules/identity/contract"
	membershipcontract "catcube/internal/modules/membership/contract"
	msgcontract "catcube/internal/modules/msg/contract"
	"catcube/internal/modules/notice"
	profilecontract "catcube/internal/modules/profile/contract"
	socialcontract "catcube/internal/modules/social/contract"
	tradecontract "catcube/internal/modules/trade/contract"
	trustcontract "catcube/internal/modules/trust/contract"
	verifycontract "catcube/internal/modules/verify/contract"
)

// 本阶段产方声明的全部事件 Topic。
func emittedTopics() []string {
	return []string{
		idcontract.EventUserRegistered, idcontract.EventUserDeactivated, idcontract.EventAuthzChanged,
		profilecontract.EventProfileUpdated, profilecontract.EventOrgCardUpdated,
		verifycontract.EventCertificationApproved, verifycontract.EventCertificationRejected,
		verifycontract.EventAffiliationVerified, verifycontract.EventOrgAdminChanged,
		contentcontract.EventContentSubmitted, contentcontract.EventContentPublished,
		contentcontract.EventContentTakenDown, contentcontract.EventContentClosed,
		trustcontract.EventModerationPassed, trustcontract.EventModerationRejected,
		trustcontract.EventReportConfirmed, trustcontract.EventViolationRecorded, trustcontract.EventAccountRestricted,
		socialcontract.EventFollowed, socialcontract.EventUnfollowed, socialcontract.EventFolloweePublished,
		tradecontract.EventOrderCreated, tradecontract.EventOrderPaid, tradecontract.EventOrderClosed, tradecontract.EventRefundSucceeded,
		membershipcontract.EventMemberActivated, membershipcontract.EventMemberExpired, membershipcontract.EventSubAccountChanged,
		msgcontract.EventMessageSent,
		// p3 cube
		cubecontract.EventCubeCreated, cubecontract.EventMemberJoined,
		cubecontract.EventMemberRemoved, cubecontract.EventCubeDissolved,
		cubecontract.EventProjectCreated, cubecontract.EventProjectClosed, cubecontract.EventProjectReopened,
		cubecontract.EventTaskAssigned, cubecontract.EventTaskSubmitted, cubecontract.EventTaskConfirmed,
		cubecontract.EventReportSubmitted, cubecontract.EventReportConfirmed, cubecontract.EventReportRejected,
		tradecontract.EventApportionFormed, tradecontract.EventApportionFailed,
		// p3 circle
		circlecontract.EventCircleCreated, circlecontract.EventCircleEnded, circlecontract.EventCircleDissolved,
		circlecontract.EventMemberJoined, circlecontract.EventDemandPublished, circlecontract.EventDemandExpired,
	}
}

// 契约测试（p3）：新增协作模块 OpenAPI 路径已声明。
func TestOpenAPIDeclaresP3Paths(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contractsDir(t, "openapi"), "mp.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	paths := []string{
		"/cubes:", "/cubes/join:", "/cubes/{id}:", "/cubes/{id}/invites:",
		"/cubes/{id}/members:", "/cubes/{id}/members/{uid}/remove:",
		"/cubes/{id}/leave:", "/cubes/{id}/transfer:", "/cubes/{id}/dissolve:",
		"/cubes/plans:", "/cubes/{id}/plan:", "/cubes/{id}/apportion:",
		"/cubes/{id}/apportion/pledge:", "/cubes/{id}/apportion/exit:", "/cube-apportions/{groupID}:",
		"/cubes/{id}/link-requests:", "/cubes/{id}/link-requests/{reqID}/decide:", "/cubes/{id}/projects/{pid}/publish:",
		"/circles:", "/circles/{id}:", "/circles/{id}/apply:", "/circles/{id}/invite:", "/circles/join:",
		"/circles/{id}/dissolve:", "/circles/{id}/demands:", "/circles/{id}/feeds:",
		"/notices:", "/notices/{id}/read:", "/notice-preferences:", "/subscribe-quota/apply:",
		"/circles/{id}/mounts:", "/circles/mounts/{mid}/decide:", "/circles/{id}/demands/{did}/to-cube:",
		"/aggregate/cube-home:", "/aggregate/circle-detail:",
	}
	for _, p := range paths {
		if !strings.Contains(text, "\n  "+p) {
			t.Fatalf("OpenAPI 缺少 p3 路径 %q", strings.TrimSuffix(p, ":"))
		}
	}
}

// 契约测试（12.3）：每个生产事件 Topic 必须有 Schema，且 Schema 的 topic 常量与文件名一致。
func TestEventContractsDeclared(t *testing.T) {
	dir := contractsDir(t, "events")
	defined := map[string]bool{}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties struct {
				Topic struct {
					Const string `json:"const"`
				} `json:"topic"`
			} `json:"properties"`
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("事件 Schema 非法 %s: %v", f.Name(), err)
		}
		want := strings.TrimSuffix(f.Name(), ".json")
		if schema.Properties.Topic.Const != want {
			t.Fatalf("Schema %s 的 topic 常量 %q 与文件名不符", f.Name(), schema.Properties.Topic.Const)
		}
		for _, req := range []string{"event_id", "topic", "occurred_at", "space_id", "payload"} {
			if !contains(schema.Required, req) {
				t.Fatalf("Schema %s 缺 required %q", f.Name(), req)
			}
		}
		defined[schema.Properties.Topic.Const] = true
	}
	for _, topic := range emittedTopics() {
		if !defined[topic] {
			t.Fatalf("生产事件 %q 缺 Schema（contracts/events/）", topic)
		}
	}
}

// 契约测试（12.3）：本阶段 OpenAPI 主链路路径均已声明。
func TestOpenAPIDeclaresP2Paths(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contractsDir(t, "openapi"), "mp.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	paths := []string{
		"/auth/wechat:", "/auth/phone:", "/users/me:", "/cards/me:", "/cards/{ref}:",
		"/certs:", "/contents:", "/contents/{id}:", "/reports:", "/report-reasons:",
		"/follows:", "/favorites:", "/orders:", "/orders/{id}/pay:", "/pay/notify:",
		"/search:", "/feed:", "/matches:", "/matches/{ref}/explain:",
		"/sessions:", "/sessions/{id}/messages:", "/sessions/{id}/read:",
		"/membership/quote:", "/membership/orders:", "/membership/me:", "/membership/sub-accounts:",
	}
	for _, p := range paths {
		if !strings.Contains(text, "\n  "+p) {
			t.Fatalf("OpenAPI 缺少主链路路径 %q", strings.TrimSuffix(p, ":"))
		}
	}
}

// 运营后台契约（admin.yaml）声明全部 admin 路由。
func TestOpenAPIDeclaresAdminPaths(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contractsDir(t, "openapi"), "admin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	paths := []string{
		"/users:", "/users/{id}/ban:", "/users/{id}/unban:",
		"/contents:", "/contents/{id}/takedown:",
		"/review-tasks:", "/review-tasks/{id}/resolve:", "/review-metrics:",
		"/orders:", "/refunds:", "/fulfillments/{orderID}/retry:",
		"/members:",
	}
	for _, p := range paths {
		if !strings.Contains(text, "\n  "+p) {
			t.Fatalf("admin OpenAPI 缺少路径 %q", strings.TrimSuffix(p, ":"))
		}
	}
}

// 通知模板覆盖（p3 §7 / 11.4）：订阅的全部业务事件均有模板，且模板数 ≥36（PRD 11.3 触达）。
func TestNoticeTemplatesCoverEvents(t *testing.T) {
	if err := notice.ValidateTemplates(); err != nil {
		t.Fatal(err)
	}
}

func contractsDir(t *testing.T, sub string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "contracts", sub)
}

func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

// 枚举契约一致性（p1 §2.3）：contracts/enums/*.json 必须与后端常量逐值一致。
// 防止「契约改了前端、后端没改」或反向漂移；新增枚举在此登记映射。
func TestEnumContractsMatchBackend(t *testing.T) {
	dir := contractsDir(t, "enums")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取枚举契约目录失败: %v", err)
	}
	// 契约枚举名 → 后端常量映射（名/值）
	backend := map[string]map[string]int64{
		"UserStatus":       {"Active": 0, "Limited": 1, "Banned": 2, "Deactivated": 3},
		"ModerationResult": {"Pending": 0, "Passed": 1, "Rejected": 2, "Retry": 3},
		"OrderStatus":      {"Pending": int64(tradecontract.StatusPending), "Paid": int64(tradecontract.StatusPaid), "Closed": int64(tradecontract.StatusClosed)},
		"ContentStatus":    {"Reviewing": int64(contentcontract.StatusReviewing), "Published": int64(contentcontract.StatusPublished), "TakenDown": int64(contentcontract.StatusTakenDown)},
		"MemberLevel":      {"VIP": int64(membershipcontract.LevelVIP), "SVIP": int64(membershipcontract.LevelSVIP), "Enterprise": int64(membershipcontract.LevelEnterprise)},
		"CubeStatus":       {"Active": int64(cubecontract.CubeActive), "Banned": int64(cubecontract.CubeBanned), "Dissolved": int64(cubecontract.CubeDissolved)},
		"CubeMemberRole":   {"None": int64(cubecontract.RoleNone), "Leader": int64(cubecontract.RoleLeader), "Honey": int64(cubecontract.RoleHoney), "Member": int64(cubecontract.RoleMember), "CoBuild": int64(cubecontract.RoleCoBuild)},
		"PlanLevel":        {"Basic": int64(cubecontract.PlanBasic), "Plus": int64(cubecontract.PlanPlus), "Premium": int64(cubecontract.PlanPremium)},
		"CircleStatus":     {"Active": int64(circlecontract.CircleActive), "Ended": int64(circlecontract.CircleEnded), "Dissolved": int64(circlecontract.CircleDissolved)},
		"CircleType":       {"Permanent": int64(circlecontract.TypePermanent), "Temporary": int64(circlecontract.TypeTemporary)},
	}
	ns := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Name   string `json:"name"`
			Values []struct {
				Key   string `json:"key"`
				Value int64  `json:"value"`
			} `json:"values"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s 解析失败: %v", e.Name(), err)
		}
		want, ok := backend[doc.Name]
		if !ok {
			t.Fatalf("契约枚举 %s 未在后端映射登记（新增枚举须同步登记）", doc.Name)
		}
		if len(want) != len(doc.Values) {
			t.Fatalf("枚举 %s 值数量不一致：契约 %d / 后端 %d", doc.Name, len(doc.Values), len(want))
		}
		for _, v := range doc.Values {
			bv, ok := want[v.Key]
			if !ok {
				t.Fatalf("枚举 %s 契约键 %s 后端缺失", doc.Name, v.Key)
			}
			if bv != v.Value {
				t.Fatalf("枚举 %s.%s 不一致：契约 %d / 后端 %d", doc.Name, v.Key, v.Value, bv)
			}
		}
		ns++
	}
	if ns < 8 {
		t.Fatalf("枚举契约过少：%d", ns)
	}
}
