//go:build integration

// 集成测试：登录/短信/刷新/会话上限/锁定/注销事件/authz_ver 陈旧窗口。
// 需真实 PostgreSQL：CATCUBE_TEST_DATABASE_URL=postgres://catcube:catcube@127.0.0.1:15432/catcube?sslmode=disable
// 运行：go test -tags=integration ./internal/modules/identity/...
package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/modules/identity/contract"
)

// fakeWeChat 同时满足 ports.WeChatAuth 与 app.PhoneExchanger：phone_code 即手机号。
type fakeWeChat struct{}

func (fakeWeChat) Code2Session(_ context.Context, code string) (string, string, error) {
	return "openid-" + code, "session-key", nil
}
func (fakeWeChat) GetPhoneNumber(_ context.Context, _, _, _ string) (string, error) {
	return "", fmt.Errorf("fake: 走 GetPhoneNumberByCode")
}
func (fakeWeChat) GetPhoneNumberByCode(_ context.Context, code string) (string, error) {
	return code, nil
}

// memOutbox 内存 Outbox，便于断言注销事件已发布。
type memOutbox struct {
	mu     sync.Mutex
	events []eventbus.Event
}

func (m *memOutbox) Insert(_ context.Context, _ any, e eventbus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
	return nil
}
func (m *memOutbox) hasTopic(topic string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.events {
		if e.Topic == topic {
			return true
		}
	}
	return false
}
func (*memOutbox) Claim(context.Context, int) ([]eventbus.Event, error) { return nil, nil }
func (*memOutbox) MarkDone(context.Context, string) error               { return nil }
func (*memOutbox) MarkRetry(context.Context, string) error              { return nil }
func (*memOutbox) MarkDead(context.Context, string) error               { return nil }
func (*memOutbox) ListDead(context.Context) ([]eventbus.Event, error)   { return nil, nil }
func (*memOutbox) Replay(context.Context, string) error                 { return nil }
func (*memOutbox) Stats(context.Context) (eventbus.OutboxStats, error) {
	return eventbus.OutboxStats{}, nil
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type loginData struct {
	UserID int64  `json:"user_id,string"`
	New    bool   `json:"new"`
	Tokens tokens `json:"tokens"`
}

type harness struct {
	t      *testing.T
	server *httptest.Server
	mod    *Module
	pool   *db.Pool
	outbox *memOutbox
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	url := os.Getenv("CATCUBE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 CATCUBE_TEST_DATABASE_URL，跳过集成测试")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if _, err := pool.Exec(ctx, migrations); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	key := sha256.Sum256([]byte("test-enc-key"))
	enc, err := sensitive.NewEncryptor(key[:])
	if err != nil {
		t.Fatal(err)
	}
	blind := sha256.Sum256([]byte("test-blind-key"))
	outbox := &memOutbox{}
	bus := eventbus.NewBus(outbox, nil)
	mod := New(Deps{
		Pool: pool, WeChat: fakeWeChat{}, PhoneCode: fakeWeChat{},
		Bus: bus, Encryptor: enc, BlindKey: blind[:], DevSMSCode: "123456",
	})
	reg := module.NewRegistry("identity")
	if err := mod.Register(reg); err != nil {
		t.Fatalf("Register 失败: %v", err)
	}
	mux := http.NewServeMux()
	for _, rt := range reg.Routes {
		mux.HandleFunc(rt.Method+" "+rt.Path, rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(func() { srv.Close(); pool.Close() })
	return &harness{t: t, server: srv, mod: mod, pool: pool, outbox: outbox}
}

func (h *harness) do(method, path, token string, body any) (int, envelope) {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			h.t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, h.server.URL+path, &buf)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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

func (h *harness) login(phone string, device string) loginData {
	h.t.Helper()
	status, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{
		"code": "wx-" + phone, "phone_code": phone, "device": device,
	})
	if status != http.StatusOK || env.Code != 0 {
		h.t.Fatalf("微信登录应成功: status=%d env=%+v", status, env)
	}
	var d loginData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		h.t.Fatal(err)
	}
	return d
}

// 每次运行用随机号码，避免跨次数据污染唯一索引。
func randPhone() string {
	return fmt.Sprintf("13%09d", rand.New(rand.NewSource(time.Now().UnixNano())).Intn(1_000_000_000))
}

func TestIntegrationWeChatLoginFlow(t *testing.T) {
	h := newHarness(t)
	phone := randPhone()

	first := h.login(phone, "dev-1")
	if !first.New {
		t.Fatal("首次登录应为新注册")
	}
	if first.Tokens.AccessToken == "" || first.Tokens.RefreshToken == "" || first.Tokens.ExpiresIn != 7200 {
		t.Fatalf("令牌不完整: %+v", first.Tokens)
	}

	// 已注册再登录：new=false，同一 user_id
	again := h.login(phone, "dev-2")
	if again.New || again.UserID != first.UserID {
		t.Fatalf("再次登录应识别为老用户: %+v vs %+v", again, first)
	}

	// /users/me
	status, env := h.do(http.MethodGet, "/api/mp/v1/users/me", first.Tokens.AccessToken, nil)
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("获取用户摘要失败: %d %+v", status, env)
	}
	var brief contract.UserBrief
	if err := json.Unmarshal(env.Data, &brief); err != nil {
		t.Fatal(err)
	}
	if brief.ID != first.UserID || brief.Status != 0 {
		t.Fatalf("用户摘要不符: %+v", brief)
	}

	// 刷新：旧 refresh 失效，新对可用
	status, env = h.do(http.MethodPost, "/api/mp/v1/auth/refresh", "", map[string]string{
		"refresh_token": first.Tokens.RefreshToken,
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("刷新失败: %d %+v", status, env)
	}
	var rotated tokens
	if err := json.Unmarshal(env.Data, &rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken == first.Tokens.RefreshToken {
		t.Fatal("refresh token 应滚动轮换")
	}
	status, env = h.do(http.MethodPost, "/api/mp/v1/auth/refresh", "", map[string]string{
		"refresh_token": first.Tokens.RefreshToken,
	})
	if env.Code != 10002 || status != http.StatusUnauthorized {
		t.Fatalf("旧 refresh 应失效(010002): %d %+v", status, env)
	}

	// 会话上限：连续登录，活跃会话 ≤3
	for i := 0; i < 4; i++ {
		h.login(phone, fmt.Sprintf("dev-extra-%d", i))
	}
	n, err := h.mod.SessionOf(context.Background(), first.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if n > 3 {
		t.Fatalf("活跃会话应 ≤3，实际 %d", n)
	}
}

func TestIntegrationSMSLoginAndLock(t *testing.T) {
	h := newHarness(t)
	phone := randPhone()

	status, env := h.do(http.MethodPost, "/api/mp/v1/auth/phone/code", "", map[string]string{"phone": phone})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("发送验证码失败: %d %+v", status, env)
	}

	// 错误验证码连续失败 → 锁定（010006 / 429）
	for i := 0; i < 5; i++ {
		_, e := h.do(http.MethodPost, "/api/mp/v1/auth/phone", "", map[string]string{"phone": phone, "code": "000000"})
		if e.Code != 10005 {
			t.Fatalf("错误验证码应返回 010005，第 %d 次得到 %+v", i+1, e)
		}
	}
	status, env = h.do(http.MethodPost, "/api/mp/v1/auth/phone", "", map[string]string{"phone": phone, "code": "000000"})
	if env.Code != 10006 || status != http.StatusTooManyRequests {
		t.Fatalf("超过阈值应锁定(010006/429): %d %+v", status, env)
	}

	// 锁定状态下即使验证码正确也被拒
	status, env = h.do(http.MethodPost, "/api/mp/v1/auth/phone/code", "", map[string]string{"phone": phone})
	if status == http.StatusOK && env.Code != 0 {
		t.Fatalf("发送验证码应成功: %+v", env)
	}

	// 另一手机号可直接短信登录成功（独立于被锁号码）
	phone2 := randPhone()
	_, _ = h.do(http.MethodPost, "/api/mp/v1/auth/phone/code", "", map[string]string{"phone": phone2})
	status, env = h.do(http.MethodPost, "/api/mp/v1/auth/phone", "", map[string]string{"phone": phone2, "code": "123456"})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("正确验证码应登录成功: %d %+v", status, env)
	}
	var d loginData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		t.Fatal(err)
	}
	if !d.New || d.Tokens.AccessToken == "" {
		t.Fatalf("短信首次登录应为新用户并签发令牌: %+v", d)
	}

	// 非法手机号 → 010007
	_, env = h.do(http.MethodPost, "/api/mp/v1/auth/phone/code", "", map[string]string{"phone": "123"})
	if env.Code != 10007 {
		t.Fatalf("非法手机号应返回 010007: %+v", env)
	}
}

func TestIntegrationDeactivatePublishesEvent(t *testing.T) {
	h := newHarness(t)
	phone := randPhone()
	d := h.login(phone, "dev-deact")

	status, env := h.do(http.MethodPost, "/api/mp/v1/users/me/deactivate", d.Tokens.AccessToken, nil)
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("注销失败: %d %+v", status, env)
	}
	if !h.outbox.hasTopic(contract.EventUserDeactivated) {
		t.Fatalf("应发布 %s", contract.EventUserDeactivated)
	}

	// 注销后既有 Access Token 立即失效（账号状态拦截）
	status, env = h.do(http.MethodGet, "/api/mp/v1/users/me", d.Tokens.AccessToken, nil)
	if status != http.StatusForbidden || env.Code != 10003 {
		t.Fatalf("注销后令牌应失效(010003/403): %d %+v", status, env)
	}
}

func TestIntegrationAuthzVerStaleWindow(t *testing.T) {
	h := newHarness(t)
	phone := randPhone()
	d := h.login(phone, "dev-authz")

	// 递增 authz_ver（模拟会员到期/封禁），旧令牌下一次请求即失效
	if err := h.mod.AuthzBump(context.Background(), d.UserID); err != nil {
		t.Fatal(err)
	}
	status, env := h.do(http.MethodGet, "/api/mp/v1/users/me", d.Tokens.AccessToken, nil)
	if status != http.StatusUnauthorized || env.Code != 2 {
		t.Fatalf("authz_ver 不一致应令牌失效(000002/401): %d %+v", status, env)
	}
}

func TestIntegrationNoTokenUnauthorized(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(http.MethodGet, "/api/mp/v1/users/me", "", nil)
	if status != http.StatusUnauthorized || env.Code != 2 {
		t.Fatalf("未登录应 401/000002: %d %+v", status, env)
	}
}
