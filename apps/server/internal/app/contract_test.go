package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	contentcontract "catcube/internal/modules/content/contract"
	cubecontract "catcube/internal/modules/cube/contract"
	idcontract "catcube/internal/modules/identity/contract"
	membershipcontract "catcube/internal/modules/membership/contract"
	msgcontract "catcube/internal/modules/msg/contract"
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
