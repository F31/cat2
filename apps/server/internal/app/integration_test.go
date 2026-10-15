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
	"os"
	"testing"
	"time"

	"catcube/internal/kernel/config"
	"catcube/internal/kernel/obs"
	"catcube/internal/modules/discovery"
	"catcube/internal/modules/membership"
	"catcube/internal/modules/trade"
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
	_, env := h.do(http.MethodGet, "/api/admin/v1/review-tasks?limit=50", reporter, nil)
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

	setCard(t, h, aTok, "张三", "310000", []string{"A01"}, []string{"N1"}, []string{"O1"})
	setCard(t, h, bTok, "李四", "310000", []string{"A01"}, []string{"O1"}, []string{"N1"})
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
	if hit == nil || hit.Score != 92.5 || len(hit.Reason) == 0 || hit.Title != "李四" {
		t.Fatalf("A feed 应命中 B（供需/行业/区域满分+身份互补 0.5）: %+v", feed.Items)
	}

	// 关键词搜索（人物）
	_, env = h.do(http.MethodGet, "/api/mp/v1/search?q=%E6%9D%8E%E5%9B%9B", "", nil)
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
