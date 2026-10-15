// Package suite 端口契约测试套件：同一套测试跑在每个适配器上（p1 §8）。
// 适配器测试文件调用对应 Run* 函数即可，新增适配器无需重写用例。
package suite

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"catcube/internal/ports"
)

// Cache 缓存端口契约。
func Cache(t *testing.T, c ports.Cache) {
	t.Helper()
	ctx := context.Background()
	if _, ok, err := c.Get(ctx, "missing"); err != nil || ok {
		t.Fatalf("未命中应 (nil,false,nil): ok=%v err=%v", ok, err)
	}
	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok, err := c.Get(ctx, "k")
	if err != nil || !ok || string(v) != "v" {
		t.Fatalf("命中失败: %q ok=%v err=%v", v, ok, err)
	}
	if err := c.Del(ctx, "k"); err != nil {
		t.Fatalf("Del: %v", err)
	}
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Fatal("删除后不应命中")
	}
	// TTL
	if err := c.Set(ctx, "ttl", []byte("x"), 20*time.Millisecond); err != nil {
		t.Fatalf("Set ttl: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok, _ := c.Get(ctx, "ttl"); ok {
		t.Fatal("TTL 过期后不应命中")
	}
}

// Queue 任务队列端口契约。
func Queue(t *testing.T, q ports.Queue) {
	t.Helper()
	ctx := context.Background()
	topic := "suite.topic." + time.Now().Format("150405.000000")
	if batch, err := q.Dequeue(ctx, topic, 10); err != nil || len(batch) != 0 {
		t.Fatalf("空队列应返回空批: %d err=%v", len(batch), err)
	}
	// 载荷按 JSON 存（PG 适配器列为 jsonb）
	if err := q.Enqueue(ctx, topic, []byte(`{"n":1}`)); err != nil {
		t.Fatalf("Enqueue 1: %v", err)
	}
	if err := q.Enqueue(ctx, topic, []byte(`{"n":2}`)); err != nil {
		t.Fatalf("Enqueue 2: %v", err)
	}
	batch, err := q.Dequeue(ctx, topic, 10)
	if err != nil || len(batch) != 2 {
		t.Fatalf("应取出 2 条: %d err=%v", len(batch), err)
	}
	seen := map[float64]bool{}
	for _, p := range batch {
		var v struct {
			N float64 `json:"n"`
		}
		if err := json.Unmarshal(p, &v); err != nil {
			t.Fatalf("载荷应为 JSON: %s err=%v", p, err)
		}
		seen[v.N] = true
	}
	if !seen[1] || !seen[2] {
		t.Fatalf("应含 n=1/2: %+v", seen)
	}
}

// Search 检索端口契约（关键词命中与删除）。
func Search(t *testing.T, s ports.Search) {
	t.Helper()
	ctx := context.Background()
	docType := "suite"
	marker := "猫方块测试词" + time.Now().Format("150405000000")
	if err := s.Index(ctx, docType, marker, map[string]any{"title": marker, "summary": "契约套件"}); err != nil {
		t.Fatalf("Index: %v", err)
	}
	ids, err := s.Query(ctx, docType, marker, 10)
	if err != nil || len(ids) == 0 {
		t.Fatalf("Query 应命中: %+v err=%v", ids, err)
	}
	if err := s.Delete(ctx, docType, marker); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// Storage 对象存储端口契约（预签名非空、Put/URL/Delete 不报错）。
func Storage(t *testing.T, st ports.Storage) {
	t.Helper()
	ctx := context.Background()
	key := "suite/obj-" + time.Now().Format("150405000000")
	url, err := st.PresignUpload(ctx, "suite", key, "image/png", time.Minute)
	if err != nil || url == "" {
		t.Fatalf("PresignUpload 应返回非空 URL: %q err=%v", url, err)
	}
	if err := st.Put(ctx, "suite", key, strings.NewReader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if u, err := st.URL(ctx, "suite", key, ""); err != nil || u == "" {
		t.Fatalf("URL 非空: %q err=%v", u, err)
	}
	if err := st.Delete(ctx, "suite", key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// Pay 支付端口契约（预支付、回调验签、退款）。
func Pay(t *testing.T, p ports.Pay) {
	t.Helper()
	ctx := context.Background()
	prepay, err := p.Prepay(ctx, ports.PayRequest{OutTradeNo: "SUITE-1", AmountFen: 100, Subject: "单测"})
	if err != nil || prepay == "" {
		t.Fatalf("Prepay 非空: %q err=%v", prepay, err)
	}
	no, amt, err := p.VerifyNotify(ctx, nil, []byte(`{"out_trade_no":"SUITE-1","amount_fen":100,"sign":"ok"}`))
	if err != nil || no != "SUITE-1" || amt != 100 {
		t.Fatalf("VerifyNotify: %q %d err=%v", no, amt, err)
	}
	if _, _, err := p.VerifyNotify(ctx, nil, []byte(`{"out_trade_no":"SUITE-1","sign":"bad"}`)); err == nil {
		t.Fatal("验签失败应报错")
	}
	if err := p.Refund(ctx, "SUITE-1", "RF-1", 100); err != nil {
		t.Fatalf("Refund: %v", err)
	}
}

// Moderation 内容安全端口契约（无 token 时跳过）。
func Moderation(t *testing.T, m ports.Moderation) {
	t.Helper()
	if m == nil {
		t.Skip("未配置内容安全适配器")
	}
	ctx := context.Background()
	if _, err := m.CheckText(ctx, ports.ModerationRequest{Text: "契约套件正常文本"}); err != nil {
		t.Skipf("内容安全接口不可用（跳过）：%v", err)
	}
}

// Realtime 实时推送端口契约（Noop 亦应返回 nil）。
func Realtime(t *testing.T, r ports.Realtime) {
	t.Helper()
	if r == nil {
		t.Skip("未配置实时适配器")
	}
	if err := r.Push(context.Background(), 1, "suite.ping", []byte(`{}`)); err != nil {
		t.Fatalf("Push 应返回 nil（一期 Noop）: %v", err)
	}
}

var _ = io.Discard
