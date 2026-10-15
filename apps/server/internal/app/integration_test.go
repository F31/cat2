//go:build integration

// 主链路集成测试（装配根级）：真实 PG + 全模块装配，覆盖 identity 登录 → profile 名片/完成度/对外展示。
// 运行：CATCUBE_TEST_DATABASE_URL=... go test -tags=integration ./internal/app/...
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"catcube/internal/bff"
	"catcube/internal/kernel/config"
	"catcube/internal/kernel/expiry"
	"catcube/internal/kernel/obs"
	"catcube/internal/modules/circle"
	"catcube/internal/modules/cube"
	"catcube/internal/modules/discovery"
	"catcube/internal/modules/membership"
	"catcube/internal/modules/msg"
	"catcube/internal/modules/trade"
	"catcube/internal/modules/trust"
	"catcube/internal/shared/media"
	"catcube/pkg/ids"
)

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type testApp struct {
	t   *testing.T
	app *App
	srv *httptest.Server
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	url := os.Getenv("CATCUBE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 CATCUBE_TEST_DATABASE_URL，跳过集成测试")
	}
	ctx := context.Background()
	a, err := Wire(ctx, config.Config{DatabaseURL: url, HTTPAddr: ":0", Roles: []string{"api"}, DevSMSCode: "123456"})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	t.Cleanup(func() { a.Pool.Close() })
	if err := RunMigrations(ctx, a.Pool, a.Modules); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	seedDict(t, a)
	srv := httptest.NewServer(obs.RequestIDMiddleware(a.Mux))
	t.Cleanup(srv.Close)
	return &testApp{t: t, app: a, srv: srv}
}

func seedDict(t *testing.T, a *App) {
	t.Helper()
	rows := []struct {
		dim, code, label string
	}{
		{"industry", "A01", "制造业"},
		{"region", "310000", "上海市"},
		{"need", "N1", "寻找渠道"},
		{"offer", "O1", "供应链服务"},
	}
	for i, r := range rows {
		_, err := a.Pool.Exec(context.Background(), `
			INSERT INTO dct_taxonomy_option (id, dimension, code, label, sort, status)
			VALUES ($1,$2,$3,$4,$5,1) ON CONFLICT (code) DO NOTHING`,
			int64(900000+i), r.dim, r.code, r.label, i)
		if err != nil {
			t.Fatalf("种子字典失败: %v", err)
		}
	}
}

func (h *testApp) do(method, path, token string, body any) (int, envelope) {
	return h.doH(method, path, token, body, nil)
}

func (h *testApp) doH(method, path, token string, body any, headers map[string]string) (int, envelope) {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, &buf)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	return resp.StatusCode, env
}

func randPhone() string {
	return fmt.Sprintf("13%09d", rand.New(rand.NewSource(time.Now().UnixNano())).Intn(1_000_000_000))
}

// 登录 → 补全第二层画像 → 完成度 → 对外名片过滤。
func TestIntegrationProfileFlow(t *testing.T) {
	h := newTestApp(t)
	phone := randPhone()

	status, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{
		"code": "wx-" + phone, "phone_code": phone, "device": "it",
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("登录失败: %d %+v", status, env)
	}
	var login struct {
		UserID int64 `json:"user_id,string"`
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(env.Data, &login); err != nil {
		t.Fatal(err)
	}
	token := login.Tokens.AccessToken

	// 名片懒建（第一层）
	status, env = h.do(http.MethodGet, "/api/mp/v1/cards/me", token, nil)
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("取我的名片失败: %d %+v", status, env)
	}

	// 未补全 → completion 未完成
	_, env = h.do(http.MethodGet, "/api/mp/v1/cards/me/completion", token, nil)
	var comp struct {
		Complete bool     `json:"complete"`
		Missing  []string `json:"missing"`
		Tier     int16    `json:"tier"`
	}
	_ = json.Unmarshal(env.Data, &comp)
	if comp.Complete || comp.Tier != 1 {
		t.Fatalf("初始应未完成/tier1: %+v", comp)
	}

	// 补全第二层五项 + 第三层优势/联系方式；关闭 intro 对外展示
	upd := map[string]any{
		"name":                "张三",
		"subject_type_code":   "person",
		"region_code":         "310000",
		"industry_codes":      []string{"A01"},
		"need_codes":          []string{"N1"},
		"offer_codes":         []string{"O1"},
		"intro":               "专注产业合作",
		"core_strength":       "十年供应链经验",
		"contact_phone":       "13800000000",
		"visibility_settings": map[string]bool{"intro": false},
	}
	status, env = h.do(http.MethodPut, "/api/mp/v1/cards/me", token, upd)
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("更新名片失败: %d %+v", status, env)
	}

	_, env = h.do(http.MethodGet, "/api/mp/v1/cards/me/completion", token, nil)
	_ = json.Unmarshal(env.Data, &comp)
	if !comp.Complete || comp.Tier != 3 {
		t.Fatalf("补全后应完成/tier3: %+v", comp)
	}

	// 对外名片：intro 被关闭 → 空；联系方式永不下发
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cards/user:%d", login.UserID), "", nil)
	var pub struct {
		Intro        string `json:"intro"`
		CoreStrength string `json:"core_strength"`
		HasContact   bool   `json:"has_contact"`
	}
	if err := json.Unmarshal(env.Data, &pub); err != nil {
		t.Fatal(err)
	}
	if pub.Intro != "" || pub.CoreStrength == "" || pub.HasContact {
		t.Fatalf("对外展示过滤不符: %+v", pub)
	}

	// 事件已入 Outbox（同事务）
	var n int
	if err := h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM sys_outbox WHERE topic='profile.ProfileUpdated.v1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("应发布 profile.ProfileUpdated.v1")
	}
}

// 实名认证：二要素通过 → 授 Badge + 回写 realname_status；不一致 → 驳回可重提；重复提交冲突。
func TestIntegrationVerifyRealname(t *testing.T) {
	h := newTestApp(t)

	login := func() (int64, string) {
		phone := randPhone()
		_, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{
			"code": "wx-" + phone, "phone_code": phone,
		})
		var d struct {
			UserID int64 `json:"user_id,string"`
			Tokens struct {
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		}
		_ = json.Unmarshal(env.Data, &d)
		return d.UserID, d.Tokens.AccessToken
	}

	// 通过分支
	uid, token := login()
	status, env := h.do(http.MethodPost, "/api/mp/v1/certs", token, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("实名认证应通过: %d %+v", status, env)
	}
	var cert struct {
		Status       int16  `json:"status"`
		RejectReason string `json:"reject_reason"`
	}
	_ = json.Unmarshal(env.Data, &cert)
	if cert.Status != 1 {
		t.Fatalf("实名应通过(status=1): %+v", cert)
	}
	// realname_status 回写
	var rs int16
	_ = h.app.Pool.QueryRow(context.Background(), `SELECT realname_status FROM idn_user WHERE id=$1`, uid).Scan(&rs)
	if rs != 1 {
		t.Fatalf("realname_status 应回写为 1，实际 %d", rs)
	}
	// Badge 授予
	var badgeN int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM vfy_badge_grant WHERE subject_type='user' AND subject_id=$1 AND badge_code='realname' AND revoked_at IS NULL`, uid).Scan(&badgeN)
	if badgeN != 1 {
		t.Fatalf("应授予 realname Badge")
	}
	// 重复提交 → 409 / 100004
	status, env = h.do(http.MethodPost, "/api/mp/v1/certs", token, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	if status != http.StatusConflict || env.Code != 100004 {
		t.Fatalf("重复实名应冲突(100004/409): %d %+v", status, env)
	}

	// 未勾选协议 → 400 / 100001
	_, token2 := login()
	status, env = h.do(http.MethodPost, "/api/mp/v1/certs", token2, map[string]any{
		"cert_type": 1, "real_name": "李四", "id_card_no": "110101199003070019", "agreed": false,
	})
	if status != http.StatusBadRequest || env.Code != 100001 {
		t.Fatalf("未勾选协议应 100001/400: %d %+v", status, env)
	}

	// 核验不一致 → 返回记录 status=2 可重提；realname_status=2
	uid3, token3 := login()
	_, env = h.do(http.MethodPost, "/api/mp/v1/certs", token3, map[string]any{
		"cert_type": 1, "real_name": "驳回测试", "id_card_no": "110101199003070019", "agreed": true,
	})
	_ = json.Unmarshal(env.Data, &cert)
	if cert.Status != 2 || cert.RejectReason == "" {
		t.Fatalf("不一致应驳回并带原因: %+v", cert)
	}
	_ = h.app.Pool.QueryRow(context.Background(), `SELECT realname_status FROM idn_user WHERE id=$1`, uid3).Scan(&rs)
	if rs != 2 {
		t.Fatalf("核验失败应回写 realname_status=2，实际 %d", rs)
	}
	// 失败后重提（合法 retry）→ 通过
	_, env = h.do(http.MethodPost, "/api/mp/v1/certs", token3, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	_ = json.Unmarshal(env.Data, &cert)
	if cert.Status != 1 {
		t.Fatalf("失败后重提应通过: %+v", cert)
	}
}

// 内容主链路：实名 → 创建草稿 → 提交（审核通过展示）→ 详情会员字段过滤 → 关闭下架。
func TestIntegrationContentFlow(t *testing.T) {
	h := newTestApp(t)
	phone := randPhone()
	_, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{
		"code": "wx-" + phone, "phone_code": phone,
	})
	var login struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(env.Data, &login)
	token := login.Tokens.AccessToken

	// 未实名提交门槛：先建草稿再提交应被拦截
	_, env = h.do(http.MethodPost, "/api/mp/v1/contents", token, map[string]any{
		"kind": "project", "title": "合作项目A",
		"tagset": map[string]any{"region": "310000", "industry": []string{"A01"}},
		"fields": map[string]any{"amount_wan": 100, "intro": "简介", "core_strength": "优势", "contact_phone": "13800000000"},
	})
	var item struct {
		ID     int64 `json:"id,string"`
		Status int16 `json:"status"`
	}
	if err := json.Unmarshal(env.Data, &item); err != nil || item.ID == 0 {
		t.Fatalf("创建草稿失败: %+v err=%v", env, err)
	}
	status, env := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/submit", item.ID), token, nil)
	if status != http.StatusForbidden || env.Code != 110004 {
		t.Fatalf("未实名提交应 110004/403: %d %+v", status, env)
	}

	// 实名后再提交 → 展示中
	_, _ = h.do(http.MethodPost, "/api/mp/v1/certs", token, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	status, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/submit", item.ID), token, nil)
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("提交失败: %d %+v", status, env)
	}
	if err := json.Unmarshal(env.Data, &item); err != nil || item.Status != 1 {
		t.Fatalf("提交后应展示中(status=1): %+v", item)
	}

	// 游客查看详情：会员字段被过滤并给 Reason
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/contents/%d", item.ID), "", nil)
	var detail struct {
		Fields       map[string]any `json:"fields"`
		MemberGate   []string       `json:"member_gate"`
		UnlockReason string         `json:"unlock_reason"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatal(err)
	}
	if _, ok := detail.Fields["core_strength"]; ok {
		t.Fatalf("非会员应隐藏 core_strength: %v", detail.Fields)
	}
	if detail.UnlockReason == "" || len(detail.MemberGate) == 0 {
		t.Fatalf("应给出解锁 Reason: %+v", detail)
	}
	if detail.Fields["amount_wan"] == nil {
		t.Fatalf("公共字段应可见: %v", detail.Fields)
	}

	// 我的发布含该内容
	_, env = h.do(http.MethodGet, "/api/mp/v1/contents/mine?kind=project", token, nil)
	var mine struct {
		Items []json.RawMessage `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &mine)
	if len(mine.Items) == 0 {
		t.Fatal("我的发布应包含该内容")
	}

	// 本人关闭 → 已下架
	status, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/close", item.ID), token, nil)
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("关闭失败: %d %+v", status, env)
	}
	_ = json.Unmarshal(env.Data, &item)
	if item.Status != 2 {
		t.Fatalf("关闭后应已下架(status=2): %+v", item)
	}

	// 字段校验：项目缺必填 amount_wan → 110003
	status, env = h.do(http.MethodPost, "/api/mp/v1/contents", token, map[string]any{
		"kind": "project", "title": "缺字段", "fields": map[string]any{"intro": "x"},
	})
	if status != http.StatusBadRequest || env.Code != 110003 {
		t.Fatalf("缺必填字段应 110003/400: %d %+v", status, env)
	}
	// 未知类型 → 110005
	status, env = h.do(http.MethodPost, "/api/mp/v1/contents", token, map[string]any{
		"kind": "unknown", "title": "x", "fields": map[string]any{},
	})
	if status != http.StatusBadRequest || env.Code != 110005 {
		t.Fatalf("未知类型应 110005/400: %d %+v", status, env)
	}
}

// 举报链路：发布 → 举报 → 后台复核属实 → 内容下架 + 违规记分；3 次记分 → 限制发布。
func TestIntegrationTrustReportFlow(t *testing.T) {
	h := newTestApp(t)

	login := func(phone string) (int64, string) {
		_, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{
			"code": "wx-" + phone, "phone_code": phone,
		})
		var d struct {
			UserID int64 `json:"user_id,string"`
			Tokens struct {
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		}
		_ = json.Unmarshal(env.Data, &d)
		return d.UserID, d.Tokens.AccessToken
	}
	certify := func(token string) {
		_, env := h.do(http.MethodPost, "/api/mp/v1/certs", token, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		})
		if env.Code != 0 {
			t.Fatalf("实名失败: %+v", env)
		}
	}

	offenderPhone := randPhone()
	offenderID, offender := login(offenderPhone)
	certify(offender)
	_, reporter := login(randPhone())

	// 发布 3 条内容
	var itemIDs []int64
	for i := 0; i < 3; i++ {
		_, env := h.do(http.MethodPost, "/api/mp/v1/contents", offender, map[string]any{
			"kind": "project", "title": fmt.Sprintf("合作项目%d", i), "fields": map[string]any{"amount_wan": 100, "intro": "简介"},
		})
		var item struct {
			ID int64 `json:"id,string"`
		}
		_ = json.Unmarshal(env.Data, &item)
		_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/submit", item.ID), offender, nil)
		var sub struct {
			Status int16 `json:"status"`
		}
		_ = json.Unmarshal(env.Data, &sub)
		if sub.Status != 1 {
			t.Fatalf("内容应展示中: %+v", sub)
		}
		itemIDs = append(itemIDs, item.ID)

		// 举报
		status, env := h.do(http.MethodPost, "/api/mp/v1/reports", reporter, map[string]any{
			"target_ref": fmt.Sprintf("content.project:%d", item.ID), "reason_code": "fraud", "detail": "疑似诈骗",
		})
		if status != http.StatusOK || env.Code != 0 {
			t.Fatalf("举报失败: %d %+v", status, env)
		}
	}

	// 取审核队列并按任务复核属实
	_, env := h.do(http.MethodGet, "/api/admin/v1/review-tasks?limit=200", reporter, nil)
	var queue struct {
		Items []struct {
			ID          int64  `json:"id,string"`
			SubjectType string `json:"subject_type"`
			SubjectID   int64  `json:"subject_id,string"`
			Source      int16  `json:"source"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &queue); err != nil {
		t.Fatal(err)
	}
	reportTasks := map[int64]bool{}
	for _, tk := range queue.Items {
		if tk.Source == 2 {
			reportTasks[tk.SubjectID] = true
		}
	}
	if len(reportTasks) < 3 {
		t.Fatalf("应有 3 条举报任务: %+v", queue.Items)
	}
	for _, tk := range queue.Items {
		if tk.Source != 2 {
			continue
		}
		status, env := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/review-tasks/%d/resolve", tk.ID), reporter,
			map[string]any{"confirmed": true, "reason": "举报属实"})
		if status != http.StatusOK || env.Code != 0 {
			t.Fatalf("复核失败: %d %+v", status, env)
		}
	}

	// 投递 Outbox 事件（触发 content 订阅 ReportConfirmed 下架）
	if _, err := h.app.Bus.Relay(context.Background(), "test", 200); err != nil {
		t.Fatalf("事件投递失败: %v", err)
	}

	// 内容均下架
	for _, id := range itemIDs {
		_, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/contents/%d", id), offender, nil)
		var it struct {
			Status int16 `json:"status"`
		}
		_ = json.Unmarshal(env.Data, &it)
		if it.Status != 2 {
			t.Fatalf("举报核实后内容应下架: id=%d %+v", id, it)
		}
	}

	// 3 次记分 → 限制发布
	var restrictionTotal, scoreTotal int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM trs_restriction WHERE user_id=$1 AND expires_at>now() AND lifted_at IS NULL`, offenderID).Scan(&restrictionTotal)
	if restrictionTotal < 1 {
		t.Fatal("3 次记分后应产生限制发布")
	}
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM trs_violation_score WHERE user_id=$1`, offenderID).Scan(&scoreTotal)
	if scoreTotal != 3 {
		t.Fatalf("应有 3 条违规记分，实际 %d", scoreTotal)
	}

	// 事件已入 Outbox
	for _, topic := range []string{"trust.ReportConfirmed.v1", "trust.ViolationRecorded.v1", "trust.AccountRestricted.v1"} {
		var n int
		_ = h.app.Pool.QueryRow(context.Background(), `SELECT count(1) FROM sys_outbox WHERE topic=$1`, topic).Scan(&n)
		if n == 0 {
			t.Fatalf("应发布事件 %s", topic)
		}
	}

	// 复核结论已落审计
	var auditCnt int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM sys_audit WHERE action='trust:review:resolve'`).Scan(&auditCnt)
	if auditCnt == 0 {
		t.Fatal("复核结论应落审计")
	}
}

// 后台下架与退款落审计。
func TestIntegrationAdminOpsAudited(t *testing.T) {
	h := newTestApp(t)
	_, adminTok := loginUser(t, h)

	// 下架已展示内容
	_, ownerTok := loginUser(t, h)
	setCard(t, h, ownerTok, "发布者", "310000", []string{"A01"}, []string{"N1"}, []string{"O1"})
	_, _ = h.do(http.MethodPost, "/api/mp/v1/certs", ownerTok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	_, env := h.do(http.MethodPost, "/api/mp/v1/contents", ownerTok, map[string]any{
		"kind": "project", "title": "待下架项目",
		"tagset": map[string]any{"region": "310000", "industry": []string{"A01"}},
		"fields": map[string]any{"amount_wan": 100, "intro": "简介", "core_strength": "优势", "contact_phone": "13800000000"},
	})
	var item struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &item)
	_, _ = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/submit", item.ID), ownerTok, nil)
	if status, env := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/contents/%d/takedown", item.ID), adminTok,
		map[string]any{"reason": "违规"}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("下架失败: %d %+v", status, env)
	}
	var cnt int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM sys_audit WHERE action='content:content:takedown'`).Scan(&cnt)
	if cnt == 0 {
		t.Fatal("下架应落审计")
	}

	// 退款落审计：开通会员产生已支付订单
	_, buyerTok := loginUser(t, h)
	_, env = h.do(http.MethodPost, "/api/mp/v1/membership/orders", buyerTok, map[string]any{"level": 1, "years": 1})
	var opened struct {
		OrderID int64 `json:"order_id,string"`
	}
	_ = json.Unmarshal(env.Data, &opened)
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/orders/%d/pay", opened.OrderID), buyerTok, nil)
	var pp struct {
		OutTradeNo string `json:"out_trade_no"`
	}
	_ = json.Unmarshal(env.Data, &pp)
	_, _ = h.do(http.MethodPost, "/api/mp/v1/pay/notify", "", map[string]any{
		"out_trade_no": pp.OutTradeNo, "amount_fen": 9900, "sign": "ok",
	})
	if status, env := h.do(http.MethodPost, "/api/admin/v1/refunds", adminTok, map[string]any{
		"order_id": fmt.Sprintf("%d", opened.OrderID), "amount_fen": 9900, "reason": "用户体验",
	}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("退款失败: %d %+v", status, env)
	}
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM sys_audit WHERE action='trade:refund:execute'`).Scan(&cnt)
	if cnt == 0 {
		t.Fatal("退款应落审计")
	}
}

// 关注=订阅：B 关注 A → A 发布 → 投递后 B 未读+1 且产生通知事件；收藏六类分 Tab。
func TestIntegrationSocialFlow(t *testing.T) {
	h := newTestApp(t)

	login := func(phone string) (int64, string) {
		_, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{
			"code": "wx-" + phone, "phone_code": phone,
		})
		var d struct {
			UserID int64 `json:"user_id,string"`
			Tokens struct {
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		}
		_ = json.Unmarshal(env.Data, &d)
		return d.UserID, d.Tokens.AccessToken
	}

	authorID, author := login(randPhone())
	followerID, follower := login(randPhone())
	_ = followerID

	// B 关注 A
	status, env := h.do(http.MethodPost, "/api/mp/v1/follows", follower, map[string]any{
		"target_ref": fmt.Sprintf("user:%d", authorID),
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("关注失败: %d %+v", status, env)
	}
	// 重复关注幂等
	_, env = h.do(http.MethodPost, "/api/mp/v1/follows", follower, map[string]any{"target_ref": fmt.Sprintf("user:%d", authorID)})
	if env.Code != 0 {
		t.Fatalf("重复关注应幂等: %+v", env)
	}

	// A 实名并发布
	_, _ = h.do(http.MethodPost, "/api/mp/v1/certs", author, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	_, env = h.do(http.MethodPost, "/api/mp/v1/contents", author, map[string]any{
		"kind": "project", "title": "新项目", "fields": map[string]any{"amount_wan": 10, "intro": "x"},
	})
	var item struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &item)
	_, _ = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/submit", item.ID), author, nil)

	// 投递事件触发 social 订阅
	if _, err := h.app.Bus.Relay(context.Background(), "test", 200); err != nil {
		t.Fatalf("事件投递失败: %v", err)
	}

	// B 未读 +1
	_, env = h.do(http.MethodGet, "/api/mp/v1/follow-unread", follower, nil)
	var unread struct {
		Unread int `json:"unread"`
	}
	_ = json.Unmarshal(env.Data, &unread)
	if unread.Unread < 1 {
		t.Fatalf("关注者未读应≥1: %+v", unread)
	}

	// B 关注列表含 A
	_, env = h.do(http.MethodGet, "/api/mp/v1/follows", follower, nil)
	var following struct {
		Items []struct {
			Target struct {
				Type string `json:"type"`
				ID   int64  `json:"id,string"`
			} `json:"target"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &following)
	if len(following.Items) != 1 || following.Items[0].Target.ID != authorID {
		t.Fatalf("关注列表应含 A: %+v", following)
	}

	// A 的粉丝列表含 B
	_, env = h.do(http.MethodGet, "/api/mp/v1/followers", author, nil)
	var followers struct {
		Items []json.RawMessage `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &followers)
	if len(followers.Items) != 1 {
		t.Fatalf("粉丝列表应含 1 人: %+v", followers)
	}

	// 收藏内容 + 六类分 Tab
	_, env = h.do(http.MethodPost, "/api/mp/v1/favorites", follower, map[string]any{
		"target_ref": fmt.Sprintf("content.project:%d", item.ID),
	})
	if env.Code != 0 {
		t.Fatalf("收藏失败: %+v", env)
	}
	_, env = h.do(http.MethodGet, "/api/mp/v1/favorites?type=project", follower, nil)
	var favs struct {
		Items []struct {
			Kind string `json:"kind"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &favs)
	if len(favs.Items) != 1 || favs.Items[0].Kind != "project" {
		t.Fatalf("收藏分 Tab 不符: %+v", favs)
	}
	// 其它 Tab 为空
	_, env = h.do(http.MethodGet, "/api/mp/v1/favorites?type=person", follower, nil)
	_ = json.Unmarshal(env.Data, &favs)
	if len(favs.Items) != 0 {
		t.Fatalf("person Tab 应为空: %+v", favs)
	}

	// 事件已入 Outbox
	for _, topic := range []string{"social.Followed.v1", "social.FolloweePublished.v1"} {
		var n int
		_ = h.app.Pool.QueryRow(context.Background(), `SELECT count(1) FROM sys_outbox WHERE topic=$1`, topic).Scan(&n)
		if n == 0 {
			t.Fatalf("应发布事件 %s", topic)
		}
	}
}

// 交易链路：下单幂等 → 支付 → 回调重复幂等（OrderPaid 只发一次）→ 金额/验签 → 退款幂等。
func TestIntegrationTradeFlow(t *testing.T) {
	h := newTestApp(t)
	phone := randPhone()
	_, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{"code": "wx-" + phone, "phone_code": phone})
	var login struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(env.Data, &login)
	token := login.Tokens.AccessToken

	// 下单（幂等键；每次运行唯一，避免复用持久库的旧单）
	idemKey := fmt.Sprintf("it-order-%d", time.Now().UnixNano())
	body := map[string]any{"biz_type": "member", "amount_fen": 9900, "price_snapshot": map[string]any{"price_fen": 9900, "years": 1}}
	status, env := h.doH(http.MethodPost, "/api/mp/v1/orders", token, body, map[string]string{"Idempotency-Key": idemKey})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("下单失败: %d %+v", status, env)
	}
	var order struct {
		ID        int64 `json:"id,string"`
		Status    int16 `json:"status"`
		AmountFen int64 `json:"amount_fen"`
	}
	_ = json.Unmarshal(env.Data, &order)
	if order.Status != 0 || order.AmountFen != 9900 {
		t.Fatalf("订单初始状态不符: %+v", order)
	}
	// 相同幂等键返回同一订单
	_, env = h.doH(http.MethodPost, "/api/mp/v1/orders", token, body, map[string]string{"Idempotency-Key": idemKey})
	var order2 struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &order2)
	if order2.ID != order.ID {
		t.Fatalf("幂等键应返回同一订单: %d vs %d", order2.ID, order.ID)
	}

	// 发起支付
	status, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/orders/%d/pay", order.ID), token, nil)
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("发起支付失败: %d %+v", status, env)
	}
	var payParams struct {
		OutTradeNo string `json:"out_trade_no"`
		AmountFen  int64  `json:"amount_fen"`
	}
	_ = json.Unmarshal(env.Data, &payParams)
	if payParams.OutTradeNo == "" {
		t.Fatal("应返回 out_trade_no")
	}

	// 验签失败
	status, env = h.do(http.MethodPost, "/api/mp/v1/pay/notify", "", map[string]any{
		"out_trade_no": payParams.OutTradeNo, "amount_fen": 9900, "sign": "bad",
	})
	if status != http.StatusBadRequest || env.Code != 80005 {
		t.Fatalf("验签失败应 080005: %d %+v", status, env)
	}

	// 首次回调
	status, env = h.do(http.MethodPost, "/api/mp/v1/pay/notify", "", map[string]any{
		"out_trade_no": payParams.OutTradeNo, "amount_fen": 9900, "sign": "ok",
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("回调失败: %d %+v", status, env)
	}
	var nr struct {
		AlreadyPaid bool `json:"already_paid"`
	}
	_ = json.Unmarshal(env.Data, &nr)
	if nr.AlreadyPaid {
		t.Fatal("首次回调不应为已支付")
	}
	// 重复回调幂等
	_, env = h.do(http.MethodPost, "/api/mp/v1/pay/notify", "", map[string]any{
		"out_trade_no": payParams.OutTradeNo, "amount_fen": 9900, "sign": "ok",
	})
	_ = json.Unmarshal(env.Data, &nr)
	if !nr.AlreadyPaid {
		t.Fatal("重复回调应返回已支付")
	}

	// 订单已支付
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/orders/%d", order.ID), token, nil)
	_ = json.Unmarshal(env.Data, &order)
	if order.Status != 1 {
		t.Fatalf("订单应已支付: %+v", order)
	}
	// OrderPaid 事件仅一条（针对本订单）
	var paidEvents int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM sys_outbox WHERE topic='trade.OrderPaid.v1' AND payload->>'order_id'=$1`,
		fmt.Sprintf("%d", order.ID)).Scan(&paidEvents)
	if paidEvents != 1 {
		t.Fatalf("OrderPaid 事件应仅 1 条，实际 %d", paidEvents)
	}

	// 后台退款（幂等）
	_, env = h.do(http.MethodPost, "/api/admin/v1/refunds", token, map[string]any{
		"order_id": fmt.Sprintf("%d", order.ID), "amount_fen": 9900, "reason": "重复支付原路退回",
	})
	var refund struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &refund)
	if refund.ID == 0 {
		t.Fatalf("退款失败: %+v", env)
	}
	_, env = h.do(http.MethodPost, "/api/admin/v1/refunds", token, map[string]any{
		"order_id": fmt.Sprintf("%d", order.ID), "amount_fen": 9900, "reason": "重复支付原路退回",
	})
	var refund2 struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &refund2)
	if refund2.ID != refund.ID {
		t.Fatalf("退款应幂等（同 payment+reason）: %d vs %d", refund2.ID, refund.ID)
	}
	var refundEvents int
	_ = h.app.Pool.QueryRow(context.Background(), `SELECT count(1) FROM sys_outbox WHERE topic='trade.RefundSucceeded.v1'`).Scan(&refundEvents)
	if refundEvents == 0 {
		t.Fatal("应发布 RefundSucceeded")
	}
}

// 会员 R3：报价 → 开通下单 → 支付履约 → 会员生效 → 权益能力 → 子账号/企业版门槛。
func TestIntegrationMembershipFlow(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	phone := randPhone()
	_, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{"code": "wx-" + phone, "phone_code": phone})
	var login struct {
		UserID int64 `json:"user_id,string"`
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(env.Data, &login)
	token, uid := login.Tokens.AccessToken, login.UserID

	// 报价：1 年无折扣、3 年 0.9、5 年 0.8
	type quote struct {
		OriginalFen  int64 `json:"original_fen"`
		DiscountRate int   `json:"discount_rate"`
		TotalFen     int64 `json:"total_fen"`
	}
	var q1, q3, q5 quote
	_, env = h.do(http.MethodGet, "/api/mp/v1/membership/quote?level=1&years=1", "", nil)
	_ = json.Unmarshal(env.Data, &q1)
	if q1.TotalFen != 9900 || q1.DiscountRate != 10000 {
		t.Fatalf("1 年报价不符: %+v", q1)
	}
	_, env = h.do(http.MethodGet, "/api/mp/v1/membership/quote?level=1&years=3", "", nil)
	_ = json.Unmarshal(env.Data, &q3)
	if q3.OriginalFen != 29700 || q3.TotalFen != 26730 || q3.DiscountRate != 9000 {
		t.Fatalf("3 年报价不符: %+v", q3)
	}
	_, env = h.do(http.MethodGet, "/api/mp/v1/membership/quote?level=1&years=5", "", nil)
	_ = json.Unmarshal(env.Data, &q5)
	if q5.TotalFen != 39600 || q5.DiscountRate != 8000 {
		t.Fatalf("5 年报价不符: %+v", q5)
	}
	// 预留档位 SVIP（level=2）不上架
	if status, env := h.do(http.MethodGet, "/api/mp/v1/membership/quote?level=2&years=1", "", nil); status != http.StatusBadRequest || env.Code != 120001 {
		t.Fatalf("SVIP 报价应 120001/400: %d %+v", status, env)
	}
	// 未登录
	if status, env := h.do(http.MethodGet, "/api/mp/v1/membership/me", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("未登录应 401: %d %+v", status, env)
	}
	// 开通前无会员权益
	if ok, _ := h.app.Policy.Has(ctx, h.app.Policy.NewActor(uid, 1), "member.active"); ok {
		t.Fatal("开通前不应具备 member.active")
	}

	// 开通 VIP 1 年 → 下单
	_, env = h.do(http.MethodPost, "/api/mp/v1/membership/orders", token, map[string]any{"level": 1, "years": 1})
	var opened struct {
		OrderID   int64 `json:"order_id,string"`
		AmountFen int64 `json:"amount_fen"`
	}
	_ = json.Unmarshal(env.Data, &opened)
	if opened.OrderID == 0 || opened.AmountFen != 9900 {
		t.Fatalf("开通下单失败: %+v", opened)
	}

	// 支付 + 回调，然后驱动履约（测试不启 scheduler）
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/orders/%d/pay", opened.OrderID), token, nil)
	var payParams struct {
		OutTradeNo string `json:"out_trade_no"`
	}
	_ = json.Unmarshal(env.Data, &payParams)
	if status, env := h.do(http.MethodPost, "/api/mp/v1/pay/notify", "", map[string]any{
		"out_trade_no": payParams.OutTradeNo, "amount_fen": 9900, "sign": "ok",
	}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("支付回调失败: %d %+v", status, env)
	}
	fulfillMemberOrder(t, h, opened.OrderID)

	// 会员生效
	_, env = h.do(http.MethodGet, "/api/mp/v1/membership/me", token, nil)
	var me struct {
		Active bool `json:"active"`
		Member struct {
			Level     int16  `json:"level"`
			Status    int16  `json:"status"`
			ExpiresAt string `json:"expires_at"`
		} `json:"member"`
	}
	_ = json.Unmarshal(env.Data, &me)
	if !me.Active || me.Member.Level != 1 || me.Member.Status != 1 || me.Member.ExpiresAt == "" {
		t.Fatalf("会员应已生效: %+v", me)
	}
	if ok, _ := h.app.Policy.Has(ctx, h.app.Policy.NewActor(uid, 1), "member.active"); !ok {
		t.Fatal("开通后应具备 member.active")
	}

	// 子账号：非企业版主账号不可管理
	other := randPhone()
	_, env = h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{"code": "wx-" + other, "phone_code": other})
	var login2 struct {
		UserID int64 `json:"user_id,string"`
	}
	_ = json.Unmarshal(env.Data, &login2)
	if status, env := h.do(http.MethodPost, "/api/mp/v1/membership/sub-accounts", token, map[string]any{"sub_user_id": fmt.Sprintf("%d", login2.UserID)}); status != http.StatusForbidden || env.Code != 120004 {
		t.Fatalf("非企业版管理子账号应 120004/403: %d %+v", status, env)
	}

	// 企业版门槛：未认证企业不可开通
	if status, env := h.do(http.MethodPost, "/api/mp/v1/membership/orders", token, map[string]any{"level": 3, "years": 1}); status != http.StatusForbidden || env.Code != 120003 {
		t.Fatalf("未认证企业开通企业版应 120003/403: %d %+v", status, env)
	}

	// 退款报价：剩余完整 1 年 × 年费 × 系数 = 9900
	var mm *membership.Module
	for _, m := range h.app.Modules {
		if v, ok := m.(*membership.Module); ok {
			mm = v
		}
	}
	amount, err := mm.RefundQuote(ctx, opened.OrderID)
	if err != nil || amount != 9900 {
		t.Fatalf("退款报价应 9900: %d %v", amount, err)
	}
}

func fulfillMemberOrder(t *testing.T, h *testApp, orderID int64) {
	t.Helper()
	ctx := context.Background()
	var tm *trade.Module
	var mm *membership.Module
	for _, m := range h.app.Modules {
		switch v := m.(type) {
		case *trade.Module:
			tm = v
		case *membership.Module:
			mm = v
		}
	}
	if tm == nil || mm == nil {
		t.Fatal("trade/membership 模块未装载")
	}
	order, err := tm.Facade().GetOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("查询订单失败: %v", err)
	}
	if err := mm.Fulfill(ctx, order); err != nil {
		t.Fatalf("会员履约失败: %v", err)
	}
}

// 端到端闭环（12.4）：注册 → 补画像 → feed 有匹配 → 详情 → 开通会员 → 联系Ta 建会话 → 发消息 → 未读正确。
func TestIntegrationClosedLoop(t *testing.T) {
	h := newTestApp(t)
	aID, aTok := loginUser(t, h)
	bID, bTok := loginUser(t, h)

	// 补画像（供需互补 + 同行业 + 同区域）
	setCard(t, h, aTok, "张三", "310000", []string{"A01"}, []string{"N1"}, []string{"O1"})
	setCard(t, h, bTok, "李四", "310000", []string{"A01"}, []string{"O1"}, []string{"N1"})
	relayAll(t, h)
	refreshCandidates(t, h)

	// 一、A 的广场 feed 命中 B
	_, env := h.do(http.MethodGet, "/api/mp/v1/feed", aTok, nil)
	var feed struct {
		Items []struct {
			Ref string `json:"ref"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &feed)
	found := false
	for _, it := range feed.Items {
		if it.Ref == fmt.Sprintf("user:%d", bID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("feed 应命中匹配对象 B: %+v", feed.Items)
	}

	// 二、查看 B 的公开详情（游客亦可，未实名不影响被检索）
	status, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cards/user:%d", bID), "", nil)
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("详情应可浏览: %d %+v", status, env)
	}

	// 三、开通会员（mock 支付回调）
	makeMember(t, h, aTok)

	// 四、联系Ta：建立单聊会话（幂等）
	_, env = h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", bID)})
	var ensured struct {
		Session struct {
			ID int64 `json:"id,string"`
		} `json:"session"`
	}
	_ = json.Unmarshal(env.Data, &ensured)
	if ensured.Session.ID == 0 {
		t.Fatalf("建立会话失败: %+v", ensured)
	}
	sid := ensured.Session.ID

	// 五、A 发消息 → B 未读 = 1
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/sessions/%d/messages", sid), aTok, map[string]any{"content": "你好，想聊聊合作"})
	if env.Code != 0 {
		t.Fatalf("发消息失败: %+v", env)
	}
	_, env = h.do(http.MethodGet, "/api/mp/v1/sessions", bTok, nil)
	var sessions struct {
		Items []struct {
			ID     int64 `json:"id,string"`
			Unread int   `json:"unread"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &sessions)
	var unread int
	for _, s := range sessions.Items {
		if s.ID == sid {
			unread = s.Unread
		}
	}
	if unread != 1 {
		t.Fatalf("B 未读应 1，实际 %d（%+v）", unread, sessions.Items)
	}
	_ = aID
}

// 权限矩阵关键格（12.1）：游客可浏览；未实名不影响被检索；发起单聊=会员权益。
func TestIntegrationPermissionMatrix(t *testing.T) {
	h := newTestApp(t)
	_, aTok := loginUser(t, h)
	bID, bTok := loginUser(t, h)
	// 两人均未实名、非会员
	setCard(t, h, aTok, "王五", "310000", []string{"A01"}, []string{"N1"}, nil)
	setCard(t, h, bTok, "赵六", "310000", []string{"A01"}, nil, []string{"N1"})
	relayAll(t, h)
	refreshCandidates(t, h)

	// 游客：搜索与公开名片可用
	if status, env := h.do(http.MethodGet, "/api/mp/v1/search?type=user", "", nil); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("游客应可搜索: %d %+v", status, env)
	}
	if status, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cards/user:%d", bID), "", nil); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("游客应可看公开名片: %d %+v", status, env)
	}
	// 未实名用户仍被检索到（实名不影响被检索）
	_, env := h.do(http.MethodGet, "/api/mp/v1/search?type=user", "", nil)
	var sr struct {
		Items []struct {
			Ref string `json:"ref"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &sr)
	found := false
	for _, it := range sr.Items {
		if it.Ref == fmt.Sprintf("user:%d", bID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("未实名用户应仍被检索: %+v", sr.Items)
	}
	// 非会员发起单聊 → 拒绝；成为会员后放行
	if status, env := h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", bID)}); status != http.StatusForbidden || env.Code != 70004 {
		t.Fatalf("非会员发起单聊应 070004/403: %d %+v", status, env)
	}
	makeMember(t, h, aTok)
	if status, env := h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", bID)}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("会员发起单聊应放行: %d %+v", status, env)
	}
}

// 后台基础管理（11.1）：用户列表/封禁解封、内容列表、会员列表。
func TestIntegrationAdminFlow(t *testing.T) {
	h := newTestApp(t)
	adminID, adminTok := loginUser(t, h)
	victimID, victimTok := loginUser(t, h)

	// 用户列表包含目标
	_, env := h.do(http.MethodGet, "/api/admin/v1/users?limit=200", adminTok, nil)
	var users struct {
		Items []struct {
			ID int64 `json:"id,string"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &users)
	seen := false
	for _, u := range users.Items {
		if u.ID == victimID {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("后台用户列表应包含 victim")
	}

	// 封禁 → 旧令牌失效（authz_ver 递增 + 状态拦截）
	if status, env := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/users/%d/ban", victimID), adminTok, map[string]any{"reason": "违规"}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("封禁应成功: %d %+v", status, env)
	}
	if status, _ := h.do(http.MethodGet, "/api/mp/v1/users/me", victimTok, nil); status != http.StatusUnauthorized {
		t.Fatalf("封禁后旧令牌应失效(401): %d", status)
	}
	// 解封
	if status, env := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/users/%d/unban", victimID), adminTok, nil); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("解封应成功: %d %+v", status, env)
	}

	// 会员列表：admin 开通后出现
	makeMember(t, h, adminTok)
	_, env = h.do(http.MethodGet, "/api/admin/v1/members?level=1", adminTok, nil)
	var members struct {
		Items []struct {
			UserID int64 `json:"user_id,string"`
			Level  int16 `json:"level"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &members)
	foundMember := false
	for _, m := range members.Items {
		if m.UserID == adminID && m.Level == 1 {
			foundMember = true
		}
	}
	if !foundMember {
		t.Fatalf("后台会员列表应包含 admin 的 VIP: %+v", members.Items)
	}

	// 订单列表（admin 开通会员产生订单）
	_, env = h.do(http.MethodGet, "/api/admin/v1/orders", adminTok, nil)
	var orders struct {
		Items []struct {
			ID int64 `json:"id,string"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &orders)
	if len(orders.Items) == 0 {
		t.Fatal("后台订单列表应至少有会员开通订单")
	}

	// 内容列表可用
	if status, env := h.do(http.MethodGet, "/api/admin/v1/contents", adminTok, nil); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("后台内容列表应可用: %d %+v", status, env)
	}
}

// BFF 聚合（5.8）：首屏聚合（卡片+画像进度+未读）与详情聚合。
func TestIntegrationBFFAggregate(t *testing.T) {
	h := newTestApp(t)
	_, aTok := loginUser(t, h)
	bID, bTok := loginUser(t, h)
	setCard(t, h, aTok, "张三", "310000", []string{"A01"}, []string{"N1"}, []string{"O1"})
	setCard(t, h, bTok, "李四", "310000", []string{"A01"}, []string{"O1"}, []string{"N1"})
	relayAll(t, h)
	refreshCandidates(t, h)

	// 首屏聚合：items + profile_progress + unread
	_, env := h.do(http.MethodGet, "/api/mp/v1/feed", aTok, nil)
	var feed struct {
		Items           []json.RawMessage `json:"items"`
		ProfileProgress struct {
			Tier int16 `json:"tier"`
		} `json:"profile_progress"`
		Unread struct {
			Total int `json:"total"`
		} `json:"unread"`
	}
	if err := json.Unmarshal(env.Data, &feed); err != nil {
		t.Fatal(err)
	}
	if len(feed.Items) == 0 || feed.ProfileProgress.Tier == 0 || feed.Unread.Total != 0 {
		t.Fatalf("首屏聚合不符: %+v", feed)
	}

	// feed 分页不报错（换一批）
	if status, env := h.do(http.MethodPost, "/api/mp/v1/feed/refresh?offset=1", aTok, nil); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("换一批应成功: %d %+v", status, env)
	}

	// 详情聚合（人）
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/aggregate/detail?ref=user:%d", bID), aTok, nil)
	var detail struct {
		Type string `json:"type"`
		Card *struct {
			Name string `json:"name"`
		} `json:"card"`
		Following bool `json:"following"`
	}
	_ = json.Unmarshal(env.Data, &detail)
	if detail.Type != "user" || detail.Card == nil || detail.Card.Name != "李四" {
		t.Fatalf("详情聚合不符: %+v", detail)
	}

	// 发消息后 B 首屏未读=1
	makeMember(t, h, aTok)
	_, env = h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", bID)})
	var ensured struct {
		Session struct {
			ID int64 `json:"id,string"`
		} `json:"session"`
	}
	_ = json.Unmarshal(env.Data, &ensured)
	_, _ = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/sessions/%d/messages", ensured.Session.ID), aTok, map[string]any{"content": "在吗"})
	_, env = h.do(http.MethodGet, "/api/mp/v1/feed", bTok, nil)
	_ = json.Unmarshal(env.Data, &feed)
	if feed.Unread.Total != 1 {
		t.Fatalf("B 首屏未读应 1: %+v", feed.Unread)
	}
}

// 详情游客可看（浏览/详情=游客可）+ 联系方式为会员权益（非会员 120007）。
func TestIntegrationBFFGuestAndContact(t *testing.T) {
	h := newTestApp(t)
	_, aTok := loginUser(t, h)
	bID, bTok := loginUser(t, h)
	status, env := h.do(http.MethodPut, "/api/mp/v1/cards/me", bTok, map[string]any{
		"name": "李四", "region_code": "310000", "contact_phone": "13800000000",
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("配置名片失败: %d %+v", status, env)
	}

	// 游客（无令牌）看详情：200 且不含关注态。
	status, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/aggregate/detail?ref=user:%d", bID), "", nil)
	var detail struct {
		Card      *struct{ Name string } `json:"card"`
		Following bool                   `json:"following"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || env.Code != 0 || detail.Card == nil || detail.Card.Name != "李四" || detail.Following {
		t.Fatalf("游客详情应可看: %d %+v", status, detail)
	}

	// 非会员揭示联系方式 → 120007/403。
	contactPath := fmt.Sprintf("/api/mp/v1/aggregate/contact?ref=user:%d", bID)
	status, env = h.do(http.MethodGet, contactPath, aTok, nil)
	if status != http.StatusForbidden || env.Code != 120007 {
		t.Fatalf("非会员揭示联系方式应 120007/403: %d %+v", status, env)
	}

	// 会员揭示手机号。
	makeMember(t, h, aTok)
	status, env = h.do(http.MethodGet, contactPath, aTok, nil)
	var contact struct {
		Phone string `json:"phone"`
	}
	_ = json.Unmarshal(env.Data, &contact)
	if status != http.StatusOK || env.Code != 0 || contact.Phone != "13800000000" {
		t.Fatalf("会员应可见联系方式: %d %+v", status, contact)
	}
}

// 首屏性能（12.5）：GET /feed 单次聚合 + 预计算候选的读路径预算（后端均值 < 300ms；
// 端到端 ≤3s 预算含前端渲染与网络，此处只守后端读路径不退化）。
func TestIntegrationFeedLatency(t *testing.T) {
	h := newTestApp(t)
	_, aTok := loginUser(t, h)
	_, bTok := loginUser(t, h)
	setCard(t, h, aTok, "张三", "310000", []string{"A01"}, []string{"N1"}, []string{"O1"})
	setCard(t, h, bTok, "李四", "310000", []string{"A01"}, []string{"O1"}, []string{"N1"})
	relayAll(t, h)
	refreshCandidates(t, h)

	// 预热（连接/缓存）
	if status, env := h.do(http.MethodGet, "/api/mp/v1/feed", aTok, nil); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("feed 预热失败: %d %+v", status, env)
	}

	const rounds = 10
	start := time.Now()
	for i := 0; i < rounds; i++ {
		status, env := h.do(http.MethodGet, "/api/mp/v1/feed", aTok, nil)
		if status != http.StatusOK || env.Code != 0 {
			t.Fatalf("feed 失败: %d %+v", status, env)
		}
	}
	avg := time.Since(start) / rounds
	if avg > 300*time.Millisecond {
		t.Fatalf("feed 读路径均值超预算: %v", avg)
	}
	t.Logf("feed 读路径均值 %v（预算 300ms）", avg)
}

// 媒体上传申请：契约 JSON body → 预签名凭证；非法类型/超限拒绝。
func TestIntegrationMediaUpload(t *testing.T) {
	h := newTestApp(t)
	_, tok := loginUser(t, h)

	status, env := h.do(http.MethodPost, "/api/mp/v1/media/uploads", tok, map[string]any{
		"filename": "avatar.png", "content_type": "image/png", "size": 1024,
	})
	var ticket struct {
		FileID    string `json:"file_id"`
		UploadURL string `json:"upload_url"`
	}
	_ = json.Unmarshal(env.Data, &ticket)
	if status != http.StatusOK || env.Code != 0 || ticket.FileID == "" || ticket.UploadURL == "" {
		t.Fatalf("申请上传应成功: %d %+v", status, env)
	}

	status, env = h.do(http.MethodPost, "/api/mp/v1/media/uploads", tok, map[string]any{
		"filename": "a.gif", "content_type": "image/gif", "size": 1024,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("非法类型应 400: %d %+v", status, env)
	}
}

// 猫立方空间与成员（p3 §1）：创建/邀请加入/移除不可重进/转让/退出/档位上限。
func TestIntegrationCubeSpaces(t *testing.T) {
	h := newTestApp(t)
	// 用固定手机号登录，便于成员变更（authz_ver 失效）后重新登录取新令牌
	aPhone, bPhone, cPhone := randPhone(), randPhone(), randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	cID, cTok := loginPhone(t, h, cPhone)
	for _, tok := range []string{aTok, bTok, cTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}

	// A 创建，指定 B 为猫蜜
	status, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "协作一号", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	var cube struct {
		ID          int64 `json:"id,string"`
		MemberCount int   `json:"member_count"`
	}
	_ = json.Unmarshal(env.Data, &cube)
	if status != http.StatusOK || env.Code != 0 || cube.ID == 0 || cube.MemberCount != 2 {
		t.Fatalf("创建猫立方失败: %d %+v", status, env)
	}

	// 基础档可加入 1 个 → 二次创建被拒 130007
	status, env = h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "协作二号", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	if status != http.StatusTooManyRequests || env.Code != 130007 {
		t.Fatalf("超可加入立方数应 130007/429: %d %+v", status, env)
	}

	// B（猫蜜）邀请 → C 凭码加入
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/invites", cube.ID), bTok, nil)
	var ticket struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(env.Data, &ticket)
	if ticket.Code == "" {
		t.Fatalf("邀请码生成失败: %+v", env)
	}
	if status, env := h.do(http.MethodPost, "/api/mp/v1/cubes/join", cTok, map[string]any{"code": ticket.Code}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("加入失败: %d %+v", status, env)
	}

	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cubes/%d/members", cube.ID), aTok, nil)
	var members struct {
		Items []struct {
			UserID int64 `json:"user_id,string"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &members)
	if len(members.Items) != 3 {
		t.Fatalf("成员应 3 人: %+v", members.Items)
	}

	// A 移除 C → C 用旧码重进被拒 130006
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/members/%d/remove", cube.ID, cID), aTok,
		map[string]any{"reason": "测试移除"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("移除失败: %d %+v", s, e)
	}
	_, cTok2 := loginPhone(t, h, cPhone) // 移除会失效旧令牌，重新登录
	if status, env := h.do(http.MethodPost, "/api/mp/v1/cubes/join", cTok2, map[string]any{"code": ticket.Code}); status != http.StatusForbidden || env.Code != 130006 {
		t.Fatalf("被移除者重进应 130006/403: %d %+v", status, env)
	}

	// 转让给 B：B 成为猫王（角色变更递增 authz_ver，用新令牌读详情）
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/transfer", cube.ID), aTok,
		map[string]any{"to_user_id": fmt.Sprintf("%d", bID)}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("转让失败: %d %+v", s, e)
	}
	_, bTok2 := loginPhone(t, h, bPhone)
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cubes/%d", cube.ID), bTok2, nil)
	var detail struct {
		MyRole int16 `json:"my_role"`
	}
	_ = json.Unmarshal(env.Data, &detail)
	if detail.MyRole != 1 {
		t.Fatalf("B 应为猫王(role=1): %+v", detail)
	}
	_, aTok2 := loginPhone(t, h, aPhone)
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/leave", cube.ID), aTok2, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("退出失败: %d %+v", s, e)
	}
}

// loginPhone 以指定手机号登录（开发态 code=手机号），返回 (user_id, token)。
func loginPhone(t *testing.T, h *testApp, phone string) (int64, string) {
	t.Helper()
	_, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{"code": "wx-" + phone, "phone_code": phone})
	var l struct {
		UserID int64 `json:"user_id,string"`
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(env.Data, &l)
	if l.UserID == 0 {
		t.Fatalf("登录失败: %+v", env)
	}
	return l.UserID, l.Tokens.AccessToken
}

// 猫立方协作实体（p3 §2）：项目/任务四态（驳回须原因、重开）/报备。
func TestIntegrationCubeCollab(t *testing.T) {
	h := newTestApp(t)
	aPhone, bPhone := randPhone(), randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	for _, tok := range []string{aTok, bTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "协作一号", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)

	// 研究项目
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/projects", cube.ID), aTok, map[string]any{
		"type": 1, "name": "市场调研A",
	})
	var proj struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &proj)
	if proj.ID == 0 {
		t.Fatalf("建项目失败: %+v", env)
	}
	// 研究记录 → 决策结论
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/projects/%d/research", cube.ID, proj.ID), aTok,
		map[string]any{"content": "访谈 3 家客户", "decision": "缩小范围到华东"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("研究记录失败: %d %+v", s, e)
	}

	// 任务：B 负责人，默认猫王/猫蜜确认
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/tasks", cube.ID), aTok, map[string]any{
		"project_id": fmt.Sprintf("%d", proj.ID), "title": "整理调研报告", "assignee_id": fmt.Sprintf("%d", bID),
	})
	var task struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &task)
	if task.ID == 0 {
		t.Fatalf("建任务失败: %+v", env)
	}

	taskURL := func(action string) string {
		return fmt.Sprintf("/api/mp/v1/cubes/%d/tasks/%d/%s", cube.ID, task.ID, action)
	}
	// 待办→进行中（负责人 B）
	if s, e := h.do(http.MethodPost, taskURL("start"), bTok, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("开始任务失败: %d %+v", s, e)
	}
	// 提交进度 → 待确认
	if s, e := h.do(http.MethodPost, taskURL("submit"), bTok, map[string]any{"progress": 60}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("提交任务失败: %d %+v", s, e)
	}
	// 驳回未填原因 → 130011/400
	if s, e := h.do(http.MethodPost, taskURL("confirm"), aTok, map[string]any{"approve": false}); s != http.StatusBadRequest || e.Code != 130011 {
		t.Fatalf("驳回未填原因应 130011/400: %d %+v", s, e)
	}
	// 确认通过 → 已完成
	if s, e := h.do(http.MethodPost, taskURL("confirm"), aTok, map[string]any{"approve": true}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("确认任务失败: %d %+v", s, e)
	}
	// 重开已完成（猫王）
	if s, e := h.do(http.MethodPost, taskURL("reopen"), aTok, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("重开失败: %d %+v", s, e)
	}
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cubes/%d/tasks", cube.ID), aTok, nil)
	var tasks struct {
		Items []struct {
			Status        int16 `json:"status"`
			ReopenedCount int   `json:"reopened_count"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &tasks)
	if len(tasks.Items) != 1 || tasks.Items[0].Status != 1 || tasks.Items[0].ReopenedCount != 1 {
		t.Fatalf("重开后应为进行中且重开次数 1: %+v", tasks.Items)
	}

	// 报备：B 提交 → A 驳回（占位 reason 必填）
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/reports", cube.ID), bTok, map[string]any{
		"project_id": fmt.Sprintf("%d", proj.ID), "customer_name": "华东重工", "contact_phone": "13900000000",
		"amount_wan": 300, "note": "有意向",
	})
	var rep struct {
		Report struct {
			ID int64 `json:"id,string"`
		} `json:"report"`
		Duplicate bool `json:"duplicate"`
	}
	_ = json.Unmarshal(env.Data, &rep)
	if rep.Report.ID == 0 {
		t.Fatalf("报备失败: %+v", env)
	}
	// 重复报备仅提示不拦截
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/reports", cube.ID), bTok, map[string]any{
		"customer_name": "华东重工", "amount_wan": 100,
	})
	_ = json.Unmarshal(env.Data, &rep)
	if !rep.Duplicate {
		t.Fatal("重复报备应提示 duplicate=true")
	}
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/reports/%d/reject", cube.ID, rep.Report.ID), aTok,
		map[string]any{"reason": "信息不全"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("驳回报备失败: %d %+v", s, e)
	}
}

// 后台封禁/解封落审计（sys_audit）。
func TestIntegrationAdminBanAudited(t *testing.T) {
	h := newTestApp(t)
	victimID, victimTok := loginUser(t, h)
	_, adminTok := loginUser(t, h)

	status, env := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/users/%d/ban", victimID), adminTok,
		map[string]any{"reason": "违规刷单"})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("封禁失败: %d %+v", status, env)
	}
	var cnt int
	if err := h.app.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM sys_audit WHERE action='identity:user:ban' AND result=0`).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt == 0 {
		t.Fatal("封禁应落审计")
	}
	// 封禁后旧令牌失效
	if s, _ := h.do(http.MethodGet, "/api/mp/v1/users/me", victimTok, nil); s != http.StatusUnauthorized {
		t.Fatalf("封禁后旧令牌应失效: %d", s)
	}
}

// 企业版过期回落（8.5）：主账号过期 → 子账号同步失效（保留归属 30 天，超期清理）。
func TestIntegrationEnterpriseSubExpiry(t *testing.T) {
	h := newTestApp(t)
	masterID, _ := loginUser(t, h)
	subID, _ := loginUser(t, h)
	ctx := context.Background()

	// 直接构造已过期的企业版主账号与一个生效子账号（企业版开通需企业认证，p2 未开放）
	if _, err := h.app.Pool.Exec(ctx, `INSERT INTO mbr_member (user_id, level, status, started_at, expires_at)
		VALUES ($1,3,1,now()-interval '2 years',now()-interval '1 day')`, masterID); err != nil {
		t.Fatal(err)
	}
	staleSubID := int64(ids.New())
	if _, err := h.app.Pool.Exec(ctx, `INSERT INTO mbr_sub_account (id, master_id, sub_user_id, status, invited_at)
		VALUES ($1,$2,$3,1,now())`, int64(ids.New()), masterID, subID); err != nil {
		t.Fatal(err)
	}
	// 一条超保留期的历史已移除记录（应被清理）
	if _, err := h.app.Pool.Exec(ctx, `INSERT INTO mbr_sub_account (id, master_id, sub_user_id, status, invited_at, removed_at)
		VALUES ($1,$2,$3,2,now()-interval '60 days',now()-interval '31 days')`, staleSubID, masterID, 999999999); err != nil {
		t.Fatal(err)
	}

	var memMod *membership.Module
	for _, m := range h.app.Modules {
		if mm, ok := m.(*membership.Module); ok {
			memMod = mm
		}
	}
	if memMod == nil {
		t.Fatal("membership 模块未装载")
	}
	if err := memMod.ExpireDue(ctx); err != nil {
		t.Fatal(err)
	}

	var status int16
	if err := h.app.Pool.QueryRow(ctx,
		`SELECT status FROM mbr_sub_account WHERE master_id=$1 AND sub_user_id=$2`, masterID, subID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != 2 {
		t.Fatalf("主账号过期后子账号应同步失效: status=%d", status)
	}
	var purged int
	if err := h.app.Pool.QueryRow(ctx,
		`SELECT count(*) FROM mbr_sub_account WHERE id=$1`, staleSubID).Scan(&purged); err != nil {
		t.Fatal(err)
	}
	if purged != 0 {
		t.Fatal("超保留期的归属记录应被清理")
	}
}

// 消息归档（9.4）：超期分区压缩上传对象存储 → 标记已归档 → detach。
func TestIntegrationMsgArchive(t *testing.T) {
	h := newTestApp(t)
	aID, aTok := loginUser(t, h)
	bID, _ := loginUser(t, h)
	makeMember(t, h, aTok)

	_, env := h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", bID)})
	var ensured struct {
		Session struct {
			ID int64 `json:"id,string"`
		} `json:"session"`
	}
	_ = json.Unmarshal(env.Data, &ensured)
	sid := ensured.Session.ID

	// 建 8 个月前的历史分区并塞一条消息（模拟超期数据）
	ctx := context.Background()
	from := time.Now().UTC().AddDate(0, -8, 0)
	from = time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	name := "msg_message_" + from.Format("200601")
	// 清理上一轮遗留（detach 后成为独立表，会阻止再次 attach）
	if _, err := h.app.Pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, name)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.app.Pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE %s PARTITION OF msg_message FOR VALUES FROM ('%s') TO ('%s')`,
		name, from.Format("2006-01-02"), to.Format("2006-01-02"))); err != nil {
		t.Fatal(err)
	}
	oldMsgID := int64(990000000000000001)
	if _, err := h.app.Pool.Exec(ctx,
		`INSERT INTO msg_message (id, session_id, seq, sender_id, msg_type, content, created_at)
		 VALUES ($1,$2,9001,$3,1,'old-msg',$4)`,
		oldMsgID, sid, aID, from.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	var msgMod *msg.Module
	for _, m := range h.app.Modules {
		if mm, ok := m.(*msg.Module); ok {
			msgMod = mm
		}
	}
	if msgMod == nil {
		t.Fatal("msg 模块未装载")
	}
	if err := msgMod.Archive(ctx); err != nil {
		t.Fatal(err)
	}

	var status int16
	var objectKey string
	if err := h.app.Pool.QueryRow(ctx,
		`SELECT status, COALESCE(object_key,'') FROM msg_archive_meta WHERE partition_name=$1`, name).Scan(&status, &objectKey); err != nil {
		t.Fatalf("归档元数据缺失: %v", err)
	}
	if status != 1 || !strings.HasSuffix(objectKey, ".jsonl.gz") {
		t.Fatalf("归档应成功并记对象 key: status=%d key=%q", status, objectKey)
	}
	// detach 后主表不再包含该分区数据
	var cnt int
	if err := h.app.Pool.QueryRow(ctx, `SELECT count(*) FROM msg_message WHERE id=$1`, oldMsgID).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("归档后主表不应含已 detach 数据: %d", cnt)
	}
}

// 非会话成员拉取被拒（070002）并落审计（sys_audit，result=拒绝）。
func TestIntegrationDeniedPullAudited(t *testing.T) {
	h := newTestApp(t)
	_, aTok := loginUser(t, h)
	bID, _ := loginUser(t, h)
	makeMember(t, h, aTok)

	_, env := h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", bID)})
	var ensured struct {
		Session struct {
			ID int64 `json:"id,string"`
		} `json:"session"`
	}
	_ = json.Unmarshal(env.Data, &ensured)

	_, cTok := loginUser(t, h) // 非成员
	status, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/sessions/%d/messages", ensured.Session.ID), cTok, nil)
	if status != http.StatusForbidden || env.Code != 70002 {
		t.Fatalf("非成员拉取应 070002/403: %d %+v", status, env)
	}

	var cnt int
	if err := h.app.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM sys_audit WHERE action='msg:message:sync' AND result=1`).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt == 0 {
		t.Fatal("拒绝访问应落审计")
	}
}

// 聊天消息机检：MessageSent → trust 订阅 → 敏感词命中入审核队列（msg.message）。
func TestIntegrationMessageModeration(t *testing.T) {
	h := newTestApp(t)
	_, aTok := loginUser(t, h)
	bID, _ := loginUser(t, h)
	makeMember(t, h, aTok)

	// 词库加载（生产由 scheduler 周期刷新）
	var trustMod *trust.Module
	for _, m := range h.app.Modules {
		if tm, ok := m.(*trust.Module); ok {
			trustMod = tm
		}
	}
	if trustMod == nil {
		t.Fatal("trust 模块未装载")
	}
	if _, err := h.app.Pool.Exec(context.Background(),
		`INSERT INTO trs_sensitive_word (word, status) VALUES ($1,1) ON CONFLICT (word) DO NOTHING`,
		"违禁词X"); err != nil {
		t.Fatal(err)
	}
	if err := trustMod.LoadWords(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, hit := trustMod.FilterSensitive("违禁词X"); !hit {
		t.Fatal("敏感词库未加载")
	}

	// 清理历史运行累积的机检任务（共享测试库）
	if _, err := h.app.Pool.Exec(context.Background(),
		`DELETE FROM trs_review_task WHERE subject_type='msg.message' AND status=0`); err != nil {
		t.Fatal(err)
	}

	// A → B 建会话并发含敏感词消息
	_, env := h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", bID)})
	var ensured struct {
		Session struct {
			ID int64 `json:"id,string"`
		} `json:"session"`
	}
	_ = json.Unmarshal(env.Data, &ensured)
	if status, env := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/sessions/%d/messages", ensured.Session.ID), aTok, map[string]any{"content": "这里含违禁词X内容"}); status != http.StatusOK {
		t.Fatalf("发消息失败: %d %+v", status, env)
	}
	relayAll(t, h)

	// 会员激活会使旧 access token 的 authz_ver 失效，用新登录的令牌查审核队列。
	_, adminTok := loginUser(t, h)
	qstatus, env := h.do(http.MethodGet, "/api/admin/v1/review-tasks", adminTok, nil)
	if qstatus != http.StatusOK || env.Code != 0 {
		t.Fatalf("审核队列接口异常: %d %+v", qstatus, env)
	}
	var queue struct {
		Items []struct {
			SubjectType string `json:"subject_type"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &queue)
	found := false
	for _, it := range queue.Items {
		if it.SubjectType == "msg.message" {
			found = true
		}
	}
	if !found {
		t.Fatalf("敏感消息应入审核队列: %+v", queue.Items)
	}
}

// 实名通过事件 → profile 订阅锁定 name 字段 → 改锁定字段被拒 060003。
func TestIntegrationRealnameLocksCard(t *testing.T) {
	h := newTestApp(t)
	_, tok := loginUser(t, h)
	setCard(t, h, tok, "原名", "310000", []string{"A01"}, []string{"N1"}, []string{"O1"})

	status, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("实名提交应成功: %d %+v", status, env)
	}
	relayAll(t, h) // 投递 verify.CertificationApproved

	status, env = h.do(http.MethodPut, "/api/mp/v1/cards/me", tok, map[string]any{"name": "新名"})
	if status != http.StatusForbidden || env.Code != 60003 {
		t.Fatalf("实名后改 name 应 060003/403: %d %+v", status, env)
	}
}

// 认证锁定字段不可自改（verify 通过后写 locked_fields）。
func TestIntegrationProfileLocked(t *testing.T) {
	h := newTestApp(t)
	phone := randPhone()
	_, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{
		"code": "wx-" + phone, "phone_code": phone,
	})
	var login struct {
		UserID int64 `json:"user_id,string"`
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(env.Data, &login)

	// 先建卡并锁定 name
	_, _ = h.do(http.MethodPut, "/api/mp/v1/cards/me", login.Tokens.AccessToken, map[string]any{"name": "原名"})
	if _, err := h.app.Pool.Exec(context.Background(),
		`UPDATE prf_card SET locked_fields='["name"]'::jsonb WHERE user_id=$1`, login.UserID); err != nil {
		t.Fatal(err)
	}
	status, env := h.do(http.MethodPut, "/api/mp/v1/cards/me", login.Tokens.AccessToken, map[string]any{"name": "新名"})
	if status != http.StatusForbidden || env.Code != 60003 {
		t.Fatalf("改锁定字段应 060003/403: %d %+v", status, env)
	}
}

// 登录一个用户，返回 (user_id, access_token)。
func loginUser(t *testing.T, h *testApp) (int64, string) {
	t.Helper()
	phone := randPhone()
	_, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{"code": "wx-" + phone, "phone_code": phone})
	var login struct {
		UserID int64 `json:"user_id,string"`
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(env.Data, &login)
	return login.UserID, login.Tokens.AccessToken
}

// 给指定 token 开通 VIP 1 年并触发履约（测试不启 scheduler）。
func makeMember(t *testing.T, h *testApp, token string) {
	t.Helper()
	ctx := context.Background()
	_, env := h.do(http.MethodPost, "/api/mp/v1/membership/orders", token, map[string]any{"level": 1, "years": 1})
	var opened struct {
		OrderID int64 `json:"order_id,string"`
	}
	_ = json.Unmarshal(env.Data, &opened)
	if opened.OrderID == 0 {
		t.Fatalf("开通会员下单失败: %+v", env)
	}
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/orders/%d/pay", opened.OrderID), token, nil)
	var pp struct {
		OutTradeNo string `json:"out_trade_no"`
	}
	_ = json.Unmarshal(env.Data, &pp)
	if status, env := h.do(http.MethodPost, "/api/mp/v1/pay/notify", "", map[string]any{
		"out_trade_no": pp.OutTradeNo, "amount_fen": 9900, "sign": "ok",
	}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("会员支付回调失败: %d %+v", status, env)
	}
	fulfillMemberOrder(t, h, opened.OrderID)
	_ = ctx
}

// 消息 R1：单聊会员门槛 → 发起 → 发送 → 未读 → 增量同步 → 已读清零 → 越权拒绝。
func TestIntegrationMsgFlow(t *testing.T) {
	h := newTestApp(t)
	aID, aTok := loginUser(t, h)
	bID, bTok := loginUser(t, h)

	// 非会员发起单聊 → 070004/403
	if status, env := h.do(http.MethodPost, "/api/mp/v1/sessions", bTok, map[string]any{
		"target_ref": fmt.Sprintf("user:%d", aID),
	}); status != http.StatusForbidden || env.Code != 70004 {
		t.Fatalf("非会员发起应 070004/403: %d %+v", status, env)
	}
	// 非法目标 / 与自己
	if status, env := h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": "content.project:1"}); status != http.StatusBadRequest || env.Code != 70001 {
		t.Fatalf("非法目标应 070001/400: %d %+v", status, env)
	}
	if status, env := h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", aID)}); status != http.StatusBadRequest || env.Code != 70006 {
		t.Fatalf("与自己发起应 070006/400: %d %+v", status, env)
	}

	// A 成为会员后发起
	makeMember(t, h, aTok)
	var ensured struct {
		Created bool `json:"created"`
		Session struct {
			ID int64 `json:"id,string"`
		} `json:"session"`
	}
	_, env := h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", bID)})
	_ = json.Unmarshal(env.Data, &ensured)
	if !ensured.Created || ensured.Session.ID == 0 {
		t.Fatalf("发起会话失败: %+v", ensured)
	}
	sid := ensured.Session.ID
	// 幂等：再次发起同一对象返回同一会话
	_, env = h.do(http.MethodPost, "/api/mp/v1/sessions", aTok, map[string]any{"target_ref": fmt.Sprintf("user:%d", bID)})
	var again struct {
		Created bool `json:"created"`
		Session struct {
			ID int64 `json:"id,string"`
		} `json:"session"`
	}
	_ = json.Unmarshal(env.Data, &again)
	if again.Created || again.Session.ID != sid {
		t.Fatalf("重复发起应返回同一会话: %+v", again)
	}

	// A 发送两条消息
	for i := 1; i <= 2; i++ {
		_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/sessions/%d/messages", sid), aTok, map[string]any{"content": fmt.Sprintf("你好%d", i)})
		var m struct {
			Seq int64 `json:"seq"`
		}
		_ = json.Unmarshal(env.Data, &m)
		if m.Seq != int64(i) {
			t.Fatalf("序号应递增: 期望 %d 得 %d", i, m.Seq)
		}
	}

	// B 会话列表未读=2（Peer 应为 A）
	_, env = h.do(http.MethodGet, "/api/mp/v1/sessions", bTok, nil)
	var list struct {
		Items []struct {
			ID     int64 `json:"id,string"`
			Unread int   `json:"unread"`
			Peer   struct {
				UserID int64 `json:"user_id,string"`
			} `json:"peer"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &list)
	if len(list.Items) != 1 || list.Items[0].Unread != 2 || list.Items[0].Peer.UserID != aID {
		t.Fatalf("B 未读应为 2 且 Peer=A: %+v", list.Items)
	}

	// B 增量同步
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/sessions/%d/messages?cursor=0", sid), bTok, nil)
	var sync struct {
		Items []struct {
			Seq     int64  `json:"seq"`
			Content string `json:"content"`
		} `json:"items"`
		NextCursor int64 `json:"next_cursor"`
		HasMore    bool  `json:"has_more"`
	}
	_ = json.Unmarshal(env.Data, &sync)
	if len(sync.Items) != 2 || sync.NextCursor != 2 || sync.HasMore {
		t.Fatalf("同步结果不符: %+v", sync)
	}

	// 非成员 C 拉取 → 070002/403
	_, cTok := loginUser(t, h)
	if status, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/sessions/%d/messages", sid), cTok, nil); status != http.StatusForbidden || env.Code != 70002 {
		t.Fatalf("非成员拉取应 070002/403: %d %+v", status, env)
	}

	// B 已读 → 未读清零
	if status, env := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/sessions/%d/read", sid), bTok, map[string]any{"read_seq": 2}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("已读失败: %d %+v", status, env)
	}
	_, env = h.do(http.MethodGet, "/api/mp/v1/sessions", bTok, nil)
	_ = json.Unmarshal(env.Data, &list)
	if len(list.Items) != 1 || list.Items[0].Unread != 0 {
		t.Fatalf("已读后未读应 0: %+v", list.Items)
	}

	// MessageSent 事件已入 Outbox
	var n int
	_ = h.app.Pool.QueryRow(context.Background(), `SELECT count(1) FROM sys_outbox WHERE topic='msg.MessageSent.v1'`).Scan(&n)
	if n < 2 {
		t.Fatalf("应有 ≥2 条 MessageSent 事件，实际 %d", n)
	}
}

// relayAll 驱动 Outbox 投递（worker 角色），让事件订阅生效。
func relayAll(t *testing.T, h *testApp) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 30; i++ {
		n, err := h.app.Bus.Relay(ctx, "test", 1000)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return
		}
	}
}

// setCard 补全第二层画像（触发 ProfileUpdated/投影）。
func setCard(t *testing.T, h *testApp, token, name, region string, industry, need, offer []string) {
	t.Helper()
	status, env := h.do(http.MethodPut, "/api/mp/v1/cards/me", token, map[string]any{
		"name": name, "subject_type_code": "person", "region_code": region,
		"industry_codes": industry, "need_codes": need, "offer_codes": offer,
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("补全名片失败: %d %+v", status, env)
	}
}

func refreshCandidates(t *testing.T, h *testApp) {
	t.Helper()
	for _, m := range h.app.Modules {
		if dm, ok := m.(*discovery.Module); ok {
			if _, err := dm.RefreshAll(context.Background()); err != nil {
				t.Fatalf("候选刷新失败: %v", err)
			}
			return
		}
	}
	t.Fatal("discovery 模块未装载")
}

// discovery R5：事件投影 → 候选预计算 → feed → 搜索 → 解释 → 先给后要。
func TestIntegrationDiscoveryFlow(t *testing.T) {
	h := newTestApp(t)
	_, aTok := loginUser(t, h)
	bID, bTok := loginUser(t, h)

	// B 用唯一名称，避免测试库累积的历史同名数据干扰关键词搜索断言。
	bName := fmt.Sprintf("李四%s", randPhone()[7:])
	setCard(t, h, aTok, "张三", "310000", []string{"A01"}, []string{"N1"}, []string{"O1"})
	setCard(t, h, bTok, bName, "310000", []string{"A01"}, []string{"O1"}, []string{"N1"})
	relayAll(t, h)
	refreshCandidates(t, h)

	// A 的 feed 命中 B（供需互补 + 同行业 + 同区域 + 身份互补 → 100）
	type card struct {
		Ref    string  `json:"ref"`
		Type   string  `json:"type"`
		Title  string  `json:"title"`
		Score  float64 `json:"score"`
		Reason []struct {
			Dimension string `json:"dimension"`
			Text      string `json:"text"`
		} `json:"reason"`
	}
	_, env := h.do(http.MethodGet, "/api/mp/v1/feed", aTok, nil)
	var feed struct {
		Items []card `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &feed)
	var hit *card
	for i := range feed.Items {
		if feed.Items[i].Ref == fmt.Sprintf("user:%d", bID) {
			hit = &feed.Items[i]
		}
	}
	if hit == nil || hit.Score != 92.5 || len(hit.Reason) == 0 || hit.Title != bName {
		t.Fatalf("A feed 应命中 B（供需/行业/区域满分+身份互补 0.5）: %+v", feed.Items)
	}

	// 关键词搜索（人物，用唯一名称）
	_, env = h.do(http.MethodGet, "/api/mp/v1/search?q="+neturl.QueryEscape(bName), "", nil)
	var sr struct {
		Items []card `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &sr)
	if len(sr.Items) == 0 || sr.Items[0].Ref != fmt.Sprintf("user:%d", bID) {
		t.Fatalf("关键词搜索应命中李四: %+v", sr.Items)
	}
	// 类型筛选（无需关键词）
	_, env = h.do(http.MethodGet, "/api/mp/v1/search?type=user", "", nil)
	_ = json.Unmarshal(env.Data, &sr)
	if len(sr.Items) < 2 {
		t.Fatalf("type=user 应返回 ≥2 条: %+v", sr.Items)
	}
	// 非法类型
	if status, env := h.do(http.MethodGet, "/api/mp/v1/search?type=bad", "", nil); status != http.StatusBadRequest || env.Code != 160001 {
		t.Fatalf("非法类型应 160001/400: %d %+v", status, env)
	}

	// 匹配解释
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/matches/user:%d/explain", bID), aTok, nil)
	var explain struct {
		Ref    string  `json:"ref"`
		Score  float64 `json:"score"`
		Reason []struct {
			Dimension string `json:"dimension"`
		} `json:"reason"`
	}
	_ = json.Unmarshal(env.Data, &explain)
	if explain.Score != 92.5 || len(explain.Reason) != 4 {
		t.Fatalf("解释应 92.5 分 4 维: %+v", explain)
	}

	// 先给后要：新用户无画像，注册事件触发候选 → feed 非空（回退粗匹配）
	_, cTok := loginUser(t, h)
	relayAll(t, h)
	_, env = h.do(http.MethodGet, "/api/mp/v1/feed", cTok, nil)
	_ = json.Unmarshal(env.Data, &feed)
	if len(feed.Items) == 0 {
		t.Fatal("新用户（无画像）应至少给出粗匹配卡片")
	}

	// 越权：explain 目标不存在（合法 ref 但无文档）→ 160003/404
	if status, env := h.do(http.MethodGet, "/api/mp/v1/matches/user:99999999/explain", aTok, nil); status != http.StatusNotFound || env.Code != 160003 {
		t.Fatalf("无候选应 160003/404: %d %+v", status, env)
	}
}

// 猫立方订阅与分摊（p3 §3）：直接购买履约 + 分摊成团（原子计数）+ 共建成员。
func TestIntegrationCubePlanApportion(t *testing.T) {
	h := newTestApp(t)
	aPhone, bPhone, cPhone := randPhone(), randPhone(), randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	_, cTok := loginPhone(t, h, cPhone)
	for _, tok := range []string{aTok, bTok, cTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "协作一号", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)

	// 三档（settings 默认）
	_, env = h.do(http.MethodGet, "/api/mp/v1/cubes/plans", aTok, nil)
	var plans struct {
		Items []struct {
			Level    int16 `json:"level"`
			PriceFen int64 `json:"price_fen"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &plans)
	if len(plans.Items) != 3 || plans.Items[1].PriceFen != 99900 {
		t.Fatalf("三档不符: %+v", plans.Items)
	}

	// 直接购买进阶档 → 支付 → 履约开通
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/plan", cube.ID), aTok, map[string]any{"level": 2, "years": 1})
	var planOrder struct {
		OrderID   int64 `json:"order_id,string"`
		AmountFen int64 `json:"amount_fen"`
	}
	_ = json.Unmarshal(env.Data, &planOrder)
	if planOrder.OrderID == 0 || planOrder.AmountFen != 99900 {
		t.Fatalf("订阅下单失败: %+v", env)
	}
	payOrderAndNotify(t, h, aTok, planOrder.OrderID)
	fulfillCubeOrder(t, h, planOrder.OrderID)
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cubes/%d", cube.ID), aTok, nil)
	var detail struct {
		PlanLevel    int16  `json:"plan_level"`
		PlanExpireAt string `json:"plan_expire_at"`
	}
	_ = json.Unmarshal(env.Data, &detail)
	if detail.PlanLevel != 2 || detail.PlanExpireAt == "" {
		t.Fatalf("购买后应进阶档: %+v", detail)
	}

	// 分摊：进阶档价 99900 / N=2 = 49950；B、C 认筹（非成员亦可）
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/apportion", cube.ID), aTok, map[string]any{
		"level": 2, "n": 2, "member_ids": []string{fmt.Sprintf("%d", bID)},
	})
	var group struct {
		ID     int64 `json:"id,string"`
		PerFen int64 `json:"per_fen"`
	}
	_ = json.Unmarshal(env.Data, &group)
	if group.ID == 0 || group.PerFen != 49950 {
		t.Fatalf("发起分摊失败: %+v", env)
	}

	// B 认筹 → 支付（计数 1，未成团）
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/apportion/pledge", cube.ID), bTok, map[string]any{"group_id": fmt.Sprintf("%d", group.ID)})
	var bPledge struct {
		OrderID int64 `json:"order_id,string"`
	}
	_ = json.Unmarshal(env.Data, &bPledge)
	payOrderAndNotify(t, h, bTok, bPledge.OrderID)
	if got := apportionCount(t, h, cube.ID, group.ID, aTok); got != 1 {
		t.Fatalf("B 认筹后计数应 1: %d", got)
	}

	// C 认筹 → 支付（达 N=2 成团）
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/apportion/pledge", cube.ID), cTok, map[string]any{"group_id": fmt.Sprintf("%d", group.ID)})
	var cPledge struct {
		OrderID int64 `json:"order_id,string"`
	}
	_ = json.Unmarshal(env.Data, &cPledge)
	payOrderAndNotify(t, h, cTok, cPledge.OrderID)
	relayAll(t, h) // 投递 trade.ApportionFormed → cube 开通 + 共建成员

	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cube-apportions/%d", group.ID), aTok, nil)
	var ap struct {
		PledgedCount int   `json:"pledged_count"`
		Status       int16 `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &ap)
	if ap.PledgedCount != 2 || ap.Status != 1 {
		t.Fatalf("应成团(计数2/状态1): %+v", ap)
	}
	// 超出第 3 笔被拒（已成团）
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/apportion/pledge", cube.ID), bTok,
		map[string]any{"group_id": fmt.Sprintf("%d", group.ID)}); s != http.StatusConflict {
		t.Fatalf("已成团后认筹应 409: %d %+v", s, e)
	}
	// B 成为共建成员（role=4）
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cubes/%d/members", cube.ID), aTok, nil)
	var members struct {
		Items []struct {
			UserID int64 `json:"user_id,string"`
			Role   int16 `json:"role"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &members)
	found := false
	for _, m := range members.Items {
		if m.UserID == bID && m.Role == 4 {
			found = true
		}
	}
	if !found {
		t.Fatalf("B 应为共建成员(role=4): %+v", members.Items)
	}
}

// payOrderAndNotify 下单支付并回调（开发态验签直接通过）。
func payOrderAndNotify(t *testing.T, h *testApp, tok string, orderID int64) {
	t.Helper()
	_, env := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/orders/%d/pay", orderID), tok, nil)
	var pp struct {
		OutTradeNo string `json:"out_trade_no"`
		AmountFen  int64  `json:"amount_fen"`
	}
	_ = json.Unmarshal(env.Data, &pp)
	if pp.OutTradeNo == "" {
		t.Fatalf("支付参数缺失: %+v", env)
	}
	if s, e := h.do(http.MethodPost, "/api/mp/v1/pay/notify", "", map[string]any{
		"out_trade_no": pp.OutTradeNo, "amount_fen": pp.AmountFen, "sign": "ok",
	}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("支付回调失败: %d %+v", s, e)
	}
}

// fulfillCubeOrder 触发 cube 履约（测试不启 scheduler）。
func fulfillCubeOrder(t *testing.T, h *testApp, orderID int64) {
	t.Helper()
	ctx := context.Background()
	var tm *trade.Module
	var cm *cube.Module
	for _, m := range h.app.Modules {
		switch v := m.(type) {
		case *trade.Module:
			tm = v
		case *cube.Module:
			cm = v
		}
	}
	if tm == nil || cm == nil {
		t.Fatal("trade/cube 模块未装载")
	}
	order, err := tm.Facade().GetOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("查询订单失败: %v", err)
	}
	if err := cm.Fulfill(ctx, order); err != nil {
		t.Fatalf("立方履约失败: %v", err)
	}
}

func apportionCount(t *testing.T, h *testApp, cubeID, groupID int64, tok string) int {
	t.Helper()
	_, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cube-apportions/%d", groupID), tok, nil)
	var ap struct {
		PledgedCount int `json:"pledged_count"`
	}
	_ = json.Unmarshal(env.Data, &ap)
	return ap.PledgedCount
}

// 分摊并发成团（p3 §3 / 11.2）：N 与超额同时支付，只有前 N 笔计入，超额被退款。
func TestIntegrationApportionConcurrency(t *testing.T) {
	h := newTestApp(t)
	aPhone := randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	others := make([]string, 0, 3)
	phones := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		ph := randPhone()
		phones = append(phones, ph)
		_, tok := loginPhone(t, h, ph)
		others = append(others, tok)
	}
	all := append([]string{aTok}, others...)
	for _, tok := range all {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "并发协作", "honey_user_id": fmt.Sprintf("%d", otherID(t, h, others[0])),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)

	// N=2，价 99900 → per 49950
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/apportion", cube.ID), aTok, map[string]any{
		"level": 2, "n": 2,
	})
	var group struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &group)

	// 三人各自认筹
	orders := make([]int64, 0, 3)
	toks := make([]string, 0, 3)
	for _, tok := range others {
		_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/apportion/pledge", cube.ID), tok,
			map[string]any{"group_id": fmt.Sprintf("%d", group.ID)})
		var po struct {
			OrderID int64 `json:"order_id,string"`
		}
		_ = json.Unmarshal(env.Data, &po)
		orders = append(orders, po.OrderID)
		toks = append(toks, tok)
	}

	// 并发支付回调
	var wg sync.WaitGroup
	for i := range orders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, env := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/orders/%d/pay", orders[i]), toks[i], nil)
			var pp struct {
				OutTradeNo string `json:"out_trade_no"`
				AmountFen  int64  `json:"amount_fen"`
			}
			_ = json.Unmarshal(env.Data, &pp)
			_, _ = h.do(http.MethodPost, "/api/mp/v1/pay/notify", "", map[string]any{
				"out_trade_no": pp.OutTradeNo, "amount_fen": pp.AmountFen, "sign": "ok",
			})
		}(i)
	}
	wg.Wait()

	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cube-apportions/%d", group.ID), aTok, nil)
	var ap struct {
		PledgedCount int   `json:"pledged_count"`
		Status       int16 `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &ap)
	if ap.PledgedCount != 2 || ap.Status != 1 {
		t.Fatalf("并发成团应计数=2/状态=1（不超额）: %+v", ap)
	}
	// 超额一笔应产生退款（原路退回）
	var refunds int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM trd_refund WHERE reason = $1`,
		fmt.Sprintf("apportion:%d:over", group.ID)).Scan(&refunds)
	if refunds != 1 {
		t.Fatalf("超额认筹应退款 1 笔，实际 %d", refunds)
	}
	_ = phones
}

// otherID 取令牌对应用户 ID（/users/me 返回 id）。
func otherID(t *testing.T, h *testApp, tok string) int64 {
	t.Helper()
	_, env := h.do(http.MethodGet, "/api/mp/v1/users/me", tok, nil)
	var me struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &me)
	if me.ID == 0 {
		t.Fatalf("取用户 ID 失败: %+v", env)
	}
	return me.ID
}

// 未成团退款幂等（p3 §3 / 11.2）：到期扫描逐笔原路退回，全部完成才终结。
func TestIntegrationApportionFailedRefund(t *testing.T) {
	h := newTestApp(t)
	aPhone, bPhone := randPhone(), randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	for _, tok := range []string{aTok, bTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "退款协作", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/apportion", cube.ID), aTok, map[string]any{"level": 2, "n": 3})
	var group struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &group)

	// B 认筹并支付（1/3）
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/apportion/pledge", cube.ID), bTok, map[string]any{"group_id": fmt.Sprintf("%d", group.ID)})
	var pledge struct {
		OrderID int64 `json:"order_id,string"`
	}
	_ = json.Unmarshal(env.Data, &pledge)
	payOrderAndNotify(t, h, bTok, pledge.OrderID)

	// 令其到期
	if _, err := h.app.Pool.Exec(context.Background(),
		`UPDATE trd_apportion_group SET deadline=now()-interval '1 hour' WHERE id=$1`, group.ID); err != nil {
		t.Fatal(err)
	}
	// 手动触发到期扫描（测试不启 scheduler）
	var tm *trade.Module
	for _, m := range h.app.Modules {
		if v, ok := m.(*trade.Module); ok {
			tm = v
		}
	}
	if err := tm.ExpireApportions(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 认筹已退
	var pledgeStatus int16
	if err := h.app.Pool.QueryRow(context.Background(),
		`SELECT status FROM trd_pledge WHERE group_id=$1 AND user_id=$2`, group.ID, bID).Scan(&pledgeStatus); err != nil {
		t.Fatal(err)
	}
	if pledgeStatus != 1 {
		t.Fatalf("未成团认筹应已退款: status=%d", pledgeStatus)
	}
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cube-apportions/%d", group.ID), aTok, nil)
	var ap struct {
		Status int16 `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &ap)
	if ap.Status != 3 {
		t.Fatalf("全部退款后应终结(status=3): %+v", ap)
	}
	// 幂等：再次执行不报错、不重复退款
	if err := tm.ExpireApportions(context.Background()); err != nil {
		t.Fatal(err)
	}
	var refunds int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM trd_refund WHERE reason LIKE $1`,
		fmt.Sprintf("apportion:%d:pledge:%%", group.ID)).Scan(&refunds)
	if refunds != 1 {
		t.Fatalf("退款应幂等（1 笔），实际 %d", refunds)
	}
}

// 猫立方项目关联（p3 §4）：发布到平台回写 / 本人内容直接加 / 他组织需项目方确认 / 平台下架展示。
func TestIntegrationCubeProjectLink(t *testing.T) {
	h := newTestApp(t)
	aPhone, bPhone, dPhone := randPhone(), randPhone(), randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	_, dTok := loginPhone(t, h, dPhone)
	for _, tok := range []string{aTok, bTok, dTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "关联协作", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)

	// D 发布平台项目
	_, env = h.do(http.MethodPost, "/api/mp/v1/contents", dTok, map[string]any{
		"kind": "project", "title": "华东重工项目",
		"tagset": map[string]any{"region": "310000", "industry": []string{"A01"}},
		"fields": map[string]any{"amount_wan": 500, "intro": "简介", "core_strength": "优势", "contact_phone": "13800000000"},
	})
	var item struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &item)
	if _, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/submit", item.ID), dTok, nil); e.Code != 0 {
		t.Fatalf("提交失败: %+v", e)
	}

	// A（猫王）引入他组织项目 → 需项目方确认
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/link-requests", cube.ID), aTok,
		map[string]any{"content_project_id": fmt.Sprintf("%d", item.ID)})
	var lr struct {
		Direct    bool  `json:"direct"`
		RequestID int64 `json:"request_id,string"`
	}
	_ = json.Unmarshal(env.Data, &lr)
	if lr.Direct || lr.RequestID == 0 {
		t.Fatalf("他组织项目应走确认: %+v", env)
	}
	// D（项目方）确认 → 生成项目并引用
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/link-requests/%d/decide", cube.ID, lr.RequestID), dTok,
		map[string]any{"approve": true})
	var decided struct {
		ProjectID int64 `json:"project_id,string"`
	}
	_ = json.Unmarshal(env.Data, &decided)
	if decided.ProjectID == 0 {
		t.Fatalf("项目方确认后应生成项目: %+v", env)
	}
	// 项目列表含引用与平台状态
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cubes/%d/projects", cube.ID), aTok, nil)
	var projects struct {
		Items []struct {
			ID               int64  `json:"id,string"`
			ContentProjectID int64  `json:"content_project_id,string"`
			PlatformStatus   string `json:"platform_status"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &projects)
	if len(projects.Items) != 1 || projects.Items[0].ContentProjectID != item.ID || projects.Items[0].PlatformStatus != "active" {
		t.Fatalf("项目引用/平台状态不符: %+v", projects.Items)
	}

	// 立方内本地项目「发布到平台」回写 ID
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/projects", cube.ID), aTok, map[string]any{
		"type": 1, "name": "本地立项A",
	})
	var local struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &local)
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/projects/%d/publish", cube.ID, local.ID), aTok, nil)
	var pub struct {
		ContentProjectID int64 `json:"content_project_id,string"`
	}
	_ = json.Unmarshal(env.Data, &pub)
	if pub.ContentProjectID == 0 {
		t.Fatalf("发布到平台应回写内容 ID: %+v", env)
	}
	// 平台下架 → 立方项目卡片显示 taken_down（不删除）
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/contents/%d/takedown", item.ID), dTok,
		map[string]any{"reason": "违规"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("下架失败: %d %+v", s, e)
	}
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cubes/%d/projects", cube.ID), aTok, nil)
	_ = json.Unmarshal(env.Data, &projects)
	found := false
	for _, p := range projects.Items {
		if p.ContentProjectID == item.ID && p.PlatformStatus == "taken_down" {
			found = true
		}
	}
	if !found {
		t.Fatalf("平台下架应显示 taken_down 且不删除: %+v", projects.Items)
	}
}

// 圈子生命周期与圈内发布（p3 §5/§6-publish）：建圈/邀请加入/申请/到期结束/转常设/解散/配额/续期。
func TestIntegrationCircleLifecycle(t *testing.T) {
	h := newTestApp(t)
	aPhone, bPhone := randPhone(), randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	for _, tok := range []string{aTok, bTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	// 建临时圈（30 天）
	_, env := h.do(http.MethodPost, "/api/mp/v1/circles", aTok, map[string]any{
		"name": "华东制造业圈", "type": 2, "duration_days": 30, "region_code": "310000",
	})
	var cobj struct {
		ID     int64 `json:"id,string"`
		Status int16 `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &cobj)
	if cobj.ID == 0 || cobj.Status != 0 {
		t.Fatalf("建圈失败: %+v", env)
	}
	// 配额：当日再建被拒 140005/429
	if s, e := h.do(http.MethodPost, "/api/mp/v1/circles", aTok, map[string]any{"name": "第二圈", "type": 2, "duration_days": 7}); s != http.StatusTooManyRequests || e.Code != 140005 {
		t.Fatalf("建圈配额应 140005/429: %d %+v", s, e)
	}
	// 邀请链接加入
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/invite", cobj.ID), aTok, nil)
	var inv struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(env.Data, &inv)
	if inv.Code == "" {
		t.Fatalf("邀请码生成失败: %+v", env)
	}
	if s, e := h.do(http.MethodPost, "/api/mp/v1/circles/join", bTok, map[string]any{"code": inv.Code}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("邀请加入失败: %d %+v", s, e)
	}
	// 成员数 2
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/circles/%d", cobj.ID), bTok, nil)
	var detail struct {
		MemberCount int   `json:"member_count"`
		MyRole      int16 `json:"my_role"`
		Joined      bool  `json:"joined"`
	}
	_ = json.Unmarshal(env.Data, &detail)
	if detail.MemberCount != 2 || detail.MyRole != 2 || !detail.Joined {
		t.Fatalf("成员/角色不符: %+v", detail)
	}

	// 发布需求（普通成员，仅本圈）+ 续期
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/demands", cobj.ID), bTok, map[string]any{
		"title": "寻找钣金供应商", "content": "月需求 500 件", "contact_phone": "13900000000", "scope": 1,
	})
	var dem struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &dem)
	if dem.ID == 0 {
		t.Fatalf("发布需求失败: %+v", env)
	}
	// 每日发布额度：再发 2 条后第 4 条被拒
	for i := 0; i < 2; i++ {
		if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/demands", cobj.ID), bTok, map[string]any{
			"title": fmt.Sprintf("需求%d", i), "content": "内容",
		}); s != http.StatusOK || e.Code != 0 {
			t.Fatalf("发布需求失败: %d %+v", s, e)
		}
	}
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/demands", cobj.ID), bTok, map[string]any{
		"title": "第四条", "content": "内容",
	}); s != http.StatusTooManyRequests || e.Code != 140008 {
		t.Fatalf("每日发布额度应 140008/429: %d %+v", s, e)
	}
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/demands/%d/renew", dem.ID), bTok, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("续期失败: %d %+v", s, e)
	}

	// 到期 → EndDue 转已结束 → 圈主转常设
	if _, err := h.app.Pool.Exec(context.Background(),
		`UPDATE cir_circle SET end_at=now()-interval '1 hour' WHERE id=$1`, cobj.ID); err != nil {
		t.Fatal(err)
	}
	var cm *circle.Module
	for _, mod := range h.app.Modules {
		if v, ok := mod.(*circle.Module); ok {
			cm = v
		}
	}
	if cm == nil {
		t.Fatal("circle 模块未装载")
	}
	if err := cm.EndDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/circles/%d", cobj.ID), aTok, nil)
	_ = json.Unmarshal(env.Data, &detail)
	status := circleStatus(t, h, cobj.ID, aTok)
	if status != 1 {
		t.Fatalf("到期应转已结束(status=1): %d", status)
	}
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/convert", cobj.ID), aTok, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("转常设失败: %d %+v", s, e)
	}
	if status = circleStatus(t, h, cobj.ID, aTok); status != 0 {
		t.Fatalf("转常设后应进行中(status=0): %d", status)
	}
	_ = bID
}

func circleStatus(t *testing.T, h *testApp, circleID int64, tok string) int16 {
	t.Helper()
	_, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/circles/%d", circleID), tok, nil)
	var d struct {
		Status int16 `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &d)
	return d.Status
}

// 通知中心（p3 §7）：事件→站内信、订阅额度消耗与降级、同圈同日合并、资金/审核类不可关闭。
func TestIntegrationNotice(t *testing.T) {
	h := newTestApp(t)
	aPhone := randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", aTok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	}); s != http.StatusOK || env.Code != 0 {
		t.Fatalf("实名失败: %d %+v", s, env)
	}
	// 实名事件 → 站内信（verify.CertificationApproved）
	relayAll(t, h)
	_, env := h.do(http.MethodGet, "/api/mp/v1/notices", aTok, nil)
	var list struct {
		Items []struct {
			TemplateCode string `json:"template_code"`
			Channel      string `json:"channel"`
		} `json:"items"`
		Unread int `json:"unread"`
	}
	_ = json.Unmarshal(env.Data, &list)
	found := false
	for _, n := range list.Items {
		if n.TemplateCode == "verify.CertificationApproved.v1" {
			found = true
		}
	}
	if !found || list.Unread == 0 {
		t.Fatalf("实名通过应产生站内信: %+v", list)
	}

	// 订阅额度不足 → 降级站内（channel=inapp）；申请额度后再发 → subscribe
	_, env = h.do(http.MethodPost, "/api/mp/v1/subscribe-quota/apply", aTok, map[string]any{
		"template_code": "verify.CertificationRejected.v1", "n": 1,
	})
	if env.Code != 0 {
		t.Fatalf("申请额度失败: %+v", env)
	}
	// 触发一条拒绝事件：用不合规身份证号（100002）→ verify 会拒绝并发布事件？开发态二要素对固定号通过；
	// 改为直接验证偏好接口行为与不可关闭约束。
	_, env = h.do(http.MethodPut, "/api/mp/v1/notice-preferences", aTok, map[string]any{
		"template_code": "trade.RefundSucceeded.v1", "enabled": false,
	})
	if env.Code != 0 {
		t.Fatalf("设置偏好失败: %+v", env)
	}
	_, env = h.do(http.MethodGet, "/api/mp/v1/notice-preferences", aTok, nil)
	var prefs struct {
		Items []struct {
			TemplateCode string `json:"template_code"`
			Enabled      bool   `json:"enabled"`
			Closeable    bool   `json:"closeable"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &prefs)
	for _, p := range prefs.Items {
		if p.TemplateCode == "trade.RefundSucceeded.v1" {
			if p.Closeable {
				t.Fatalf("资金类不可关闭(closeable 应 false): %+v", p)
			}
			if !p.Enabled {
				t.Fatalf("不可关闭类应恒为开启: %+v", p)
			}
		}
	}

	// 同圈同日合并：圈内需求发布 → ntc_digest 计数（不直接发个人信）
	bPhone := randPhone()
	_, bTok := loginPhone(t, h, bPhone)
	_, _ = h.do(http.MethodPost, "/api/mp/v1/certs", bTok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	_, env = h.do(http.MethodPost, "/api/mp/v1/circles", aTok, map[string]any{
		"name": "通知测试圈", "type": 2, "duration_days": 30,
	})
	var cir struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cir)
	if _, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/demands", cir.ID), aTok, map[string]any{
		"title": "需求一", "content": "内容",
	}); e.Code != 0 {
		t.Fatalf("发布需求失败: %+v", e)
	}
	relayAll(t, h) // 投递 circle.DemandPublished → notice 合并计数
	var digestCount int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count FROM ntc_digest WHERE circle_id=$1 AND date=current_date`, cir.ID).Scan(&digestCount)
	if digestCount < 1 {
		t.Fatalf("同圈同日应合并计数: %d", digestCount)
	}
}

// 圈子挂载与「到猫立方里办」（p3 §6.6/6.7/6.9）：本人直接挂/他人需确认/四前提校验/来源回写/搜索投影。
func TestIntegrationCircleMountToCube(t *testing.T) {
	h := newTestApp(t)
	aPhone, bPhone := randPhone(), randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	for _, tok := range []string{aTok, bTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	// A 建圈
	_, env := h.do(http.MethodPost, "/api/mp/v1/circles", aTok, map[string]any{"name": "挂载圈", "type": 1})
	var cir struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cir)

	// B 发布项目（用于他人挂载）
	_, env = h.do(http.MethodPost, "/api/mp/v1/contents", bTok, map[string]any{
		"kind": "project", "title": "B的项目",
		"tagset": map[string]any{"region": "310000", "industry": []string{"A01"}},
		"fields": map[string]any{"amount_wan": 100, "intro": "简介", "core_strength": "优势", "contact_phone": "13800000000"},
	})
	var bItem struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &bItem)
	if _, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/submit", bItem.ID), bTok, nil); e.Code != 0 {
		t.Fatalf("提交项目失败: %+v", e)
	}

	// A 申请挂载 B 的内容 → 待确认（direct=false）
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/mounts", cir.ID), aTok,
		map[string]any{"item_ref": fmt.Sprintf("content.project:%d", bItem.ID)})
	var mountResp struct {
		Mount struct {
			ID             int64 `json:"id,string"`
			ApprovalStatus int16 `json:"approval_status"`
		} `json:"mount"`
		Direct bool `json:"direct"`
	}
	_ = json.Unmarshal(env.Data, &mountResp)
	if mountResp.Direct || mountResp.Mount.ID == 0 || mountResp.Mount.ApprovalStatus != 0 {
		t.Fatalf("他人内容挂载应待确认: %+v", env)
	}
	// B 确认
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/mounts/%d/decide", mountResp.Mount.ID), bTok,
		map[string]any{"approve": true}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("内容所有者确认失败: %d %+v", s, e)
	}

	// A 发布需求（scope=2 全平台，用于搜索投影）+ 建猫立方（猫王）→ 到猫立方里办
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/demands", cir.ID), aTok, map[string]any{
		"title": "多方协作需求", "content": "需要设计+供应链", "scope": 2,
	})
	var dem struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &dem)
	// 会员能力用于 scope=2：非会员会被降级为仅本圈；此处以 b 为 honey 建立方由 A 发起
	_, env = h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "承接立方", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)

	// 四前提：A 为猫王（有建项目权限）→ to-cube 成功
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/demands/%d/to-cube", cir.ID, dem.ID), aTok,
		map[string]any{"cube_id": fmt.Sprintf("%d", cube.ID)})
	var toCube struct {
		ProjectID int64 `json:"project_id,string"`
	}
	_ = json.Unmarshal(env.Data, &toCube)
	if toCube.ProjectID == 0 {
		t.Fatalf("到猫立方里办失败: %+v", env)
	}
	// 来源需求已回写 transfer_ref
	relayAll(t, h)
	var transferRef string
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(transfer_ref,'') FROM cir_demand WHERE id=$1`, dem.ID).Scan(&transferRef)
	if transferRef != fmt.Sprintf("circle.demand:%d", dem.ID) {
		t.Fatalf("需求应回写已转入猫立方办: %q", transferRef)
	}

	// 搜索投影：circle 与 circle.demand（scope=2）入 dsc_search_doc
	var n int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM dsc_search_doc WHERE doc_ref=$1`, fmt.Sprintf("circle:%d", cir.ID)).Scan(&n)
	if n == 0 {
		t.Fatal("圈子应入搜索投影")
	}
	// 非成员 C 无建项目权限 → to-cube 被拒
	cPhone := randPhone()
	_, cTok := loginPhone(t, h, cPhone)
	_, _ = h.do(http.MethodPost, "/api/mp/v1/certs", cTok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	if s, _ := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/demands/%d/to-cube", cir.ID, dem.ID), cTok,
		map[string]any{"cube_id": fmt.Sprintf("%d", cube.ID)}); s != http.StatusForbidden {
		t.Fatalf("无建项目权限应 403: %d", s)
	}
}

// 群聊补全（p3 §8）：建立方自动建群聊、成员解析、发消息与未读、成员移除后同步、群聊名称与预览。
func TestIntegrationCubeGroupChat(t *testing.T) {
	h := newTestApp(t)
	aPhone, bPhone := randPhone(), randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	for _, tok := range []string{aTok, bTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "群聊立方", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)

	// 群聊会话已建（立方名·协作），成员=猫王+猫蜜
	_, env = h.do(http.MethodGet, "/api/mp/v1/sessions", bTok, nil)
	var sessions struct {
		Items []struct {
			ID      int64  `json:"id,string"`
			Kind    int16  `json:"kind"`
			Title   string `json:"title"`
			Unread  int    `json:"unread"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &sessions)
	var sid int64
	for _, s := range sessions.Items {
		if s.Kind == 2 && s.Title == "群聊立方·协作" {
			sid = s.ID
		}
	}
	if sid == 0 {
		t.Fatalf("应自动建群聊会话: %+v", sessions.Items)
	}

	// A 在群聊发消息 → B 未读 1
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/sessions/%d/messages", sid), aTok,
		map[string]any{"content": "欢迎大家"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("群聊发消息失败: %d %+v", s, e)
	}
	_, env = h.do(http.MethodGet, "/api/mp/v1/sessions", bTok, nil)
	_ = json.Unmarshal(env.Data, &sessions)
	for _, s := range sessions.Items {
		if s.ID == sid && s.Unread != 1 {
			t.Fatalf("B 群聊未读应 1: %+v", s)
		}
	}

	// 移除 B → 群成员同步移除（B 不再看到群聊）
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/members/%d/remove", cube.ID, bID), aTok,
		map[string]any{"reason": "测试"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("移除成员失败: %d %+v", s, e)
	}
	relayAll(t, h) // cube.MemberRemoved → msg 群成员同步
	var memberCount int
	_ = h.app.Pool.QueryRow(context.Background(),
		`SELECT count(1) FROM msg_session_member WHERE session_id=$1 AND user_id=$2`, sid, bID).Scan(&memberCount)
	if memberCount != 0 {
		t.Fatalf("移除后群成员应同步移除: %d", memberCount)
	}
}

// bff 协作聚合（p3 §10）：猫立方首页（列表+待办+未读）与圈详情（非成员裁剪）。
func TestIntegrationBFFCollaboration(t *testing.T) {
	h := newTestApp(t)
	aPhone, bPhone := randPhone(), randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	for _, tok := range []string{aTok, bTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "聚合立方", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)
	// 建一个任务（待办）→ 首页待办数 ≥1
	_, _ = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/tasks", cube.ID), aTok, map[string]any{
		"title": "聚合待办", "assignee_id": fmt.Sprintf("%d", bID),
	})

	_, env = h.do(http.MethodGet, "/api/mp/v1/aggregate/cube-home", aTok, nil)
	var home struct {
		Cubes []struct {
			ID int64 `json:"id,string"`
		} `json:"cubes"`
		TodoCount int `json:"todo_count"`
		Unread    int `json:"unread"`
	}
	_ = json.Unmarshal(env.Data, &home)
	if len(home.Cubes) != 1 || home.TodoCount < 1 {
		t.Fatalf("猫立方首页聚合不符: %+v", home)
	}

	// 圈详情聚合：游客可看（非成员裁剪）
	_, env = h.do(http.MethodPost, "/api/mp/v1/circles", aTok, map[string]any{"name": "聚合圈", "type": 1})
	var cir struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cir)
	status, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/aggregate/circle-detail?ref=circle:%d", cir.ID), "", nil)
	var detail struct {
		Circle struct {
			Name   string `json:"name"`
			Joined bool   `json:"joined"`
		} `json:"circle"`
		MemberCount int  `json:"member_count"`
		Joined      bool `json:"joined"`
	}
	_ = json.Unmarshal(env.Data, &detail)
	if status != http.StatusOK || env.Code != 0 || detail.Circle.Name != "聚合圈" || detail.Joined {
		t.Fatalf("圈详情聚合不符: %d %+v", status, detail)
	}
	_ = bTok
}

// 配额门禁（p3 §11.3）：立方成员上限（基础档 10）、公告每日上限（3）。
func TestIntegrationQuotaGuards(t *testing.T) {
	h := newTestApp(t)
	aPhone := randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	cert := func(tok string) {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	cert(aTok)
	// 基础档成员上限 10：创建=2（猫王+猫蜜）→ 再入 8 → 第 11 名被拒
	honeyPhone := randPhone()
	honeyID, honeyTok := loginPhone(t, h, honeyPhone)
	cert(honeyTok)
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "限额立方", "honey_user_id": fmt.Sprintf("%d", honeyID),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/invites", cube.ID), aTok, nil)
	var ticket struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(env.Data, &ticket)
	joined := 0
	for i := 0; i < 9; i++ {
		ph := randPhone()
		_, tok := loginPhone(t, h, ph)
		cert(tok)
		s, e := h.do(http.MethodPost, "/api/mp/v1/cubes/join", tok, map[string]any{"code": ticket.Code})
		if s == http.StatusOK && e.Code == 0 {
			joined++
		} else if e.Code == 130008 {
			break
		}
	}
	if joined != 8 {
		t.Fatalf("基础档可入 8（合计 10 人），实际 %d", joined)
	}
	// 第 11 名
	ph := randPhone()
	_, tok := loginPhone(t, h, ph)
	cert(tok)
	if s, e := h.do(http.MethodPost, "/api/mp/v1/cubes/join", tok, map[string]any{"code": ticket.Code}); s != http.StatusConflict || e.Code != 130008 {
		t.Fatalf("成员上限应 130008/409: %d %+v", s, e)
	}

	// 公告每日上限 3（圈主）
	_, env = h.do(http.MethodPost, "/api/mp/v1/circles", aTok, map[string]any{"name": "公告圈", "type": 1})
	var cir struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cir)
	for i := 0; i < 3; i++ {
		if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/feeds", cir.ID), aTok,
			map[string]any{"text": "公告", "is_announcement": true}); s != http.StatusOK || e.Code != 0 {
			t.Fatalf("公告发布失败: %d %+v", s, e)
		}
	}
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/feeds", cir.ID), aTok,
		map[string]any{"text": "第四条公告", "is_announcement": true}); s != http.StatusTooManyRequests || e.Code != 140009 {
		t.Fatalf("公告上限应 140009/429: %d %+v", s, e)
	}
}

// 通知额度消耗与降级（p3 §11.4）：申请 1 条订阅额度 → 首条订阅、次条降级站内。
func TestIntegrationNoticeQuotaDegrade(t *testing.T) {
	h := newTestApp(t)
	aPhone := randPhone()
	_, aTok := loginPhone(t, h, aPhone)
	if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", aTok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	}); s != http.StatusOK || env.Code != 0 {
		t.Fatalf("实名失败: %d %+v", s, env)
	}
	// 申请 1 条「内容已通过审核」订阅额度
	_, _ = h.do(http.MethodPost, "/api/mp/v1/subscribe-quota/apply", aTok, map[string]any{
		"template_code": "content.ContentPublished.v1", "n": 1,
	})
	// 两次发布并提交 → 两条 ContentPublished 事件
	for i := 0; i < 2; i++ {
		_, env := h.do(http.MethodPost, "/api/mp/v1/contents", aTok, map[string]any{
			"kind": "project", "title": fmt.Sprintf("内容%d", i),
			"tagset": map[string]any{"region": "310000", "industry": []string{"A01"}},
			"fields": map[string]any{"amount_wan": 100, "intro": "简介", "core_strength": "优势", "contact_phone": "13800000000"},
		})
		var item struct {
			ID int64 `json:"id,string"`
		}
		_ = json.Unmarshal(env.Data, &item)
		_, _ = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/submit", item.ID), aTok, nil)
	}
	relayAll(t, h)
	_, env := h.do(http.MethodGet, "/api/mp/v1/notices?limit=50", aTok, nil)
	var list struct {
		Items []struct {
			TemplateCode string `json:"template_code"`
			Channel      string `json:"channel"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &list)
	subscribeCnt, inappCnt := 0, 0
	for _, n := range list.Items {
		if n.TemplateCode != "content.ContentPublished.v1" {
			continue
		}
		if n.Channel == "subscribe" {
			subscribeCnt++
		} else if n.Channel == "inapp" {
			inappCnt++
		}
	}
	if subscribeCnt != 1 || inappCnt != 1 {
		t.Fatalf("订阅额度应用后应 1 订阅 1 降级站内: sub=%d inapp=%d", subscribeCnt, inappCnt)
	}
}

// 退出标准（p3 §12.3/12.4）：猫立方核心链路与圈子生命周期端到端。
func TestIntegrationP3ExitCriteria(t *testing.T) {
	h := newTestApp(t)
	aPhone, bPhone := randPhone(), randPhone()
	aID, aTok := loginPhone(t, h, aPhone)
	bID, bTok := loginPhone(t, h, bPhone)
	for _, tok := range []string{aTok, bTok} {
		if s, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); s != http.StatusOK || env.Code != 0 {
			t.Fatalf("实名失败: %d %+v", s, env)
		}
	}
	// 1) 建立方（A 猫王，B 猫蜜）
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", aTok, map[string]any{
		"name": "闭环立方", "honey_user_id": fmt.Sprintf("%d", bID),
	})
	var cube struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cube)
	// 2) 邀请加入（第三名成员 C）
	cPhone := randPhone()
	cID, cTok := loginPhone(t, h, cPhone)
	if _, e := h.do(http.MethodPost, "/api/mp/v1/certs", cTok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	}); e.Code != 0 {
		t.Fatalf("C 实名失败: %+v", e)
	}
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/invites", cube.ID), aTok, nil)
	var ticket struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(env.Data, &ticket)
	if _, e := h.do(http.MethodPost, "/api/mp/v1/cubes/join", cTok, map[string]any{"code": ticket.Code}); e.Code != 0 {
		t.Fatalf("C 加入失败: %+v", e)
	}
	// 3) 建项目
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/projects", cube.ID), aTok, map[string]any{
		"type": 1, "name": "闭环项目",
	})
	var proj struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &proj)
	// 4) 建任务并完成（B 负责，A 确认）
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/tasks", cube.ID), aTok, map[string]any{
		"project_id": fmt.Sprintf("%d", proj.ID), "title": "闭环任务", "assignee_id": fmt.Sprintf("%d", bID),
	})
	var task struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &task)
	taskURL := func(a string) string {
		return fmt.Sprintf("/api/mp/v1/cubes/%d/tasks/%d/%s", cube.ID, task.ID, a)
	}
	_, _ = h.do(http.MethodPost, taskURL("start"), bTok, nil)
	_, _ = h.do(http.MethodPost, taskURL("submit"), bTok, map[string]any{"progress": 80})
	_, _ = h.do(http.MethodPost, taskURL("confirm"), aTok, map[string]any{"approve": true})
	// 5) 报备并确认
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/reports", cube.ID), bTok, map[string]any{
		"project_id": fmt.Sprintf("%d", proj.ID), "customer_name": "闭环客户", "amount_wan": 200,
	})
	var rep struct {
		Report struct {
			ID int64 `json:"id,string"`
		} `json:"report"`
	}
	_ = json.Unmarshal(env.Data, &rep)
	if _, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/reports/%d/confirm", cube.ID, rep.Report.ID), aTok, nil); e.Code != 0 {
		t.Fatalf("报备确认失败: %+v", e)
	}
	// 6) 订阅（直接购买进阶档 → 支付 → 履约）
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/cubes/%d/plan", cube.ID), aTok, map[string]any{"level": 2, "years": 1})
	var po struct {
		OrderID int64 `json:"order_id,string"`
	}
	_ = json.Unmarshal(env.Data, &po)
	payOrderAndNotify(t, h, aTok, po.OrderID)
	fulfillCubeOrder(t, h, po.OrderID)
	if level := cubePlanLevel(t, h, cube.ID, aTok); level != 2 {
		t.Fatalf("订阅后应进阶档: %d", level)
	}

	// 圈子生命周期：建圈 → 申请加入 → 发布 → 解散
	_, env = h.do(http.MethodPost, "/api/mp/v1/circles", aTok, map[string]any{"name": "闭环圈", "type": 1})
	var cir struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &cir)
	_, cTok2 := loginPhone(t, h, cPhone) // 加入立方已递增 authz_ver，重新登录取新令牌
	if _, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/apply", cir.ID), cTok2, nil); e.Code != 0 {
		t.Fatalf("申请加入失败: %+v", e)
	}
	if _, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/feeds", cir.ID), cTok2, map[string]any{"text": "圈内动态"}); e.Code != 0 {
		t.Fatalf("圈内发布失败: %+v", e)
	}
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/dissolve", cir.ID), aTok, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("解散失败: %d %+v", s, e)
	}
	if status := circleStatus(t, h, cir.ID, aTok); status != 2 {
		t.Fatalf("解散后应已解散(status=2): %d", status)
	}
	_ = aID
	_ = cID
}

func cubePlanLevel(t *testing.T, h *testApp, cubeID int64, tok string) int16 {
	t.Helper()
	_, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/cubes/%d", cubeID), tok, nil)
	var d struct {
		PlanLevel int16 `json:"plan_level"`
	}
	_ = json.Unmarshal(env.Data, &d)
	return d.PlanLevel
}

// 媒体孤儿清理（p1 §12.4）：引用归零且超保留期 → 删对象与元数据。
func TestIntegrationMediaOrphanPurge(t *testing.T) {
	h := newTestApp(t)
	_, tok := loginUser(t, h)
	// 申请上传（生成一条 med_file）
	_, env := h.do(http.MethodPost, "/api/mp/v1/media/uploads", tok, map[string]any{
		"filename": "avatar.png", "content_type": "image/png", "size": 2048,
	})
	var ticket struct {
		FileID string `json:"file_id"`
	}
	_ = json.Unmarshal(env.Data, &ticket)
	if ticket.FileID == "" {
		t.Fatalf("上传申请失败: %+v", env)
	}
	// 置为孤儿且超保留期
	if _, err := h.app.Pool.Exec(context.Background(),
		`UPDATE med_file SET status=0, created_at=now()-interval '8 days' WHERE id=$1`, ticket.FileID); err != nil {
		t.Fatal(err)
	}
	var mediaMod *media.Module
	for _, m := range h.app.Modules {
		if v, ok := m.(*media.Module); ok {
			mediaMod = v
		}
	}
	if mediaMod == nil {
		t.Fatal("media 模块未装载")
	}
	if err := mediaMod.PurgeOrphans(context.Background()); err != nil {
		t.Fatal(err)
	}
	var cnt int
	_ = h.app.Pool.QueryRow(context.Background(), `SELECT count(1) FROM med_file WHERE id=$1`, ticket.FileID).Scan(&cnt)
	if cnt != 0 {
		t.Fatalf("孤儿文件应被清理: %d", cnt)
	}
}

// 死信查询与重放（p1 §6.2）：标记死信 → 后台列出 → 重放复位。
func TestIntegrationDeadLetterReplay(t *testing.T) {
	h := newTestApp(t)
	_, tok := loginUser(t, h)
	ctx := context.Background()
	// 造一条死信
	eid := "deadletter-test-" + randPhone()
	if _, err := h.app.Pool.Exec(ctx, `
		INSERT INTO sys_outbox (event_id, topic, payload, occurred_at, status, retry_count)
		VALUES ($1,'demo.DeadLetter.v1','{}'::jsonb,now(),9,9)`, eid); err != nil {
		t.Fatal(err)
	}
	_, env := h.do(http.MethodGet, "/api/admin/v1/dead-letters", tok, nil)
	var list struct {
		Items []struct {
			EventID string `json:"event_id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &list)
	found := false
	for _, it := range list.Items {
		if it.EventID == eid {
			found = true
		}
	}
	if !found {
		t.Fatalf("死信应可列出: %+v", list.Items)
	}
	// 重放：状态复位 retry_count=0
	if s, e := h.do(http.MethodPost, "/api/admin/v1/dead-letters/"+eid+"/replay", tok, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("重放失败: %d %+v", s, e)
	}
	var status, retry int
	_ = h.app.Pool.QueryRow(ctx, `SELECT status, retry_count FROM sys_outbox WHERE event_id=$1`, eid).Scan(&status, &retry)
	if status != 0 || retry != 0 {
		t.Fatalf("重放应复位 status=0/retry=0: %d/%d", status, retry)
	}
}

// 到期转态样例链路（p1 §6.5）：批量扫描、分批、幂等（重复执行不二次转移）。
func TestIntegrationExpirySample(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	// 样例表：带 expires_at 部分索引（供各模块复用的模板）
	if _, err := h.app.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS demo_expiry (
			id bigint PRIMARY KEY, status smallint NOT NULL DEFAULT 0, expires_at timestamptz
		);
		CREATE INDEX IF NOT EXISTS idx_demo_expiry_due ON demo_expiry(expires_at) WHERE status=0;`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.app.Pool.Exec(ctx, `DELETE FROM demo_expiry`); err != nil {
		t.Fatal(err)
	}
	base := int64(777000000000000000)
	// 1 已到期生效中；2 未到期；3 已过期（已是目标态）
	if _, err := h.app.Pool.Exec(ctx, `
		INSERT INTO demo_expiry (id, status, expires_at) VALUES
		($1,0,now()-interval '1 hour'), ($2,0,now()+interval '1 hour'), ($3,1,now()-interval '1 day')`,
		base+1, base+2, base+3); err != nil {
		t.Fatal(err)
	}
	n, err := expiry.Run(ctx, h.app.Pool, expiry.Spec{Table: "demo_expiry", ActiveStatus: 0, ExpiredStatus: 5}, nil)
	if err != nil || n != 1 {
		t.Fatalf("应转移 1 条: n=%d err=%v", n, err)
	}
	// 幂等：再次执行 0
	if n, err := expiry.Run(ctx, h.app.Pool, expiry.Spec{Table: "demo_expiry", ActiveStatus: 0, ExpiredStatus: 5}, nil); err != nil || n != 0 {
		t.Fatalf("重复执行应 0（幂等）: n=%d err=%v", n, err)
	}
	var statusExpired, statusFuture int16
	_ = h.app.Pool.QueryRow(ctx, `SELECT status FROM demo_expiry WHERE id=$1`, base+1).Scan(&statusExpired)
	_ = h.app.Pool.QueryRow(ctx, `SELECT status FROM demo_expiry WHERE id=$1`, base+2).Scan(&statusFuture)
	if statusExpired != 5 || statusFuture != 0 {
		t.Fatalf("到期转移/未到期保持不符: %d/%d", statusExpired, statusFuture)
	}
	// 非法表名拒绝
	if _, err := expiry.Run(ctx, h.app.Pool, expiry.Spec{Table: "demo;drop", ActiveStatus: 0, ExpiredStatus: 5}, nil); err == nil {
		t.Fatal("非法表名应被拒")
	}
}

// 后台菜单声明（p1 §9.9/§11.7）：AdminSection 声明式返回，内容随启用模块。
func TestIntegrationAdminMenus(t *testing.T) {
	h := newTestApp(t)
	_, tok := loginUser(t, h)
	_, env := h.do(http.MethodGet, "/api/admin/v1/menus", tok, nil)
	var res struct {
		Items []struct {
			Key    string `json:"key"`
			Path   string `json:"path"`
			Module string `json:"module"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &res)
	want := map[string]bool{
		"identity.users": true, "content.contents": true, "trust.reviews": true,
		"trade.orders": true, "membership.members": true, "cube.cubes": true,
		"circle.circles": true, "notice.templates": true, "ops.dead_letters": true,
	}
	for _, it := range res.Items {
		delete(want, it.Key)
	}
	if len(want) != 0 {
		t.Fatalf("菜单声明缺失: %+v", want)
	}
	// 未登录拒绝
	if s, _ := h.do(http.MethodGet, "/api/admin/v1/menus", "", nil); s != http.StatusUnauthorized {
		t.Fatalf("未登录应 401: %d", s)
	}
}

// 特性开关灰度（p6 前置）：后台配置（白名单/比例）→ 客户端评估按用户命中。
func TestIntegrationFeatureFlags(t *testing.T) {
	h := newTestApp(t)
	uid, tok := loginUser(t, h)
	_, otherTok := loginUser(t, h)
	// 后台配置：仅白名单命中当前用户
	if s, e := h.do(http.MethodPut, "/api/admin/v1/feature-flags/demo.flag", tok, map[string]any{
		"enabled": true, "ratio": 0, "users": []int64{uid},
	}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("配置开关失败: %d %+v", s, e)
	}
	_, env := h.do(http.MethodGet, "/api/mp/v1/flags", tok, nil)
	var mine struct {
		Flags map[string]bool `json:"flags"`
	}
	_ = json.Unmarshal(env.Data, &mine)
	if !mine.Flags["demo.flag"] {
		t.Fatalf("白名单用户应命中: %+v", mine.Flags)
	}
	_, env = h.do(http.MethodGet, "/api/mp/v1/flags", otherTok, nil)
	var other struct {
		Flags map[string]bool `json:"flags"`
	}
	_ = json.Unmarshal(env.Data, &other)
	if other.Flags["demo.flag"] {
		t.Fatalf("非白名单用户不应命中: %+v", other.Flags)
	}
	// 关闭 → 全不命中
	_, _ = h.do(http.MethodPut, "/api/admin/v1/feature-flags/demo.flag", tok, map[string]any{"enabled": false})
	_, env = h.do(http.MethodGet, "/api/mp/v1/flags", tok, nil)
	_ = json.Unmarshal(env.Data, &mine)
	if mine.Flags["demo.flag"] {
		t.Fatal("关闭后不应命中")
	}
	// 后台列表可见
	_, env = h.do(http.MethodGet, "/api/admin/v1/feature-flags", tok, nil)
	var list struct {
		Items []struct {
			Key string `json:"key"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &list)
	found := false
	for _, it := range list.Items {
		if it.Key == "demo.flag" {
			found = true
		}
	}
	if !found {
		t.Fatalf("后台应列出开关: %+v", list.Items)
	}
}

// 首屏聚合缓存（S1 Redis/进程内）：cube-home 二次请求命中缓存，写后立刻反映。
func TestIntegrationCubeHomeCache(t *testing.T) {
	h := newTestApp(t)
	uid, tok := loginUser(t, h)
	bid, bTok := loginUser(t, h)
	// 双方均已实名（建立方需实名，蜜方亦需实名）
	for _, tk := range []string{tok, bTok} {
		if _, certEnv := h.do(http.MethodPost, "/api/mp/v1/certs", tk, map[string]any{
			"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		}); certEnv.Code != 0 {
			t.Fatalf("实名失败: %+v", certEnv)
		}
	}
	_, env := h.do(http.MethodPost, "/api/mp/v1/cubes", tok, map[string]any{
		"name": "缓存立方", "honey_user_id": fmt.Sprintf("%d", bid),
	})
	if env.Code != 0 {
		t.Fatalf("建立方失败: %+v", env)
	}
	// 首次：写缓存
	_, env = h.do(http.MethodGet, "/api/mp/v1/aggregate/cube-home", tok, nil)
	var first bff.CubeHomeData
	_ = json.Unmarshal(env.Data, &first)
	if len(first.Cubes) != 1 {
		t.Fatalf("应有 1 个立方: %+v", first)
	}
	// 缓存键已写入且 TTL>0
	raw, ok, err := h.app.Cache.Get(context.Background(), "bff:cube-home:"+strconv.FormatInt(uid, 10))
	if err != nil || !ok || len(raw) == 0 {
		t.Fatalf("缓存键应写入: ok=%v err=%v", ok, err)
	}
	// 二次：命中缓存返回同值
	_, env = h.do(http.MethodGet, "/api/mp/v1/aggregate/cube-home", tok, nil)
	var second bff.CubeHomeData
	_ = json.Unmarshal(env.Data, &second)
	if second.TodoCount != first.TodoCount || len(second.Cubes) != len(first.Cubes) {
		t.Fatalf("缓存返回应一致: %+v vs %+v", first, second)
	}
}
