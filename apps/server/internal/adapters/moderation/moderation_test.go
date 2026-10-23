package moderation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"catcube/internal/ports"
)

func testClient(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := NewClient(func(context.Context) (string, error) { return "token-1", nil })
	c.BaseURL = srv.URL
	return c, srv.Close
}

func TestCheckTextPass(t *testing.T) {
	var body map[string]any
	c, closeFn := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wxa/msg_sec_check" || r.URL.Query().Get("access_token") != "token-1" {
			t.Fatalf("请求错误: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"errcode":0,"result":{"suggest":"pass","label":100}}`))
	})
	defer closeFn()
	res, err := c.CheckText(context.Background(), ports.ModerationRequest{Text: "正常文本"})
	if err != nil || !res.Pass {
		t.Fatalf("应通过: %+v %v", res, err)
	}
	if body["content"] != "正常文本" || int(body["version"].(float64)) != 2 {
		t.Fatalf("请求体错误: %+v", body)
	}
}

func TestCheckTextReviewAndRiskyRejected(t *testing.T) {
	for _, suggest := range []string{"review", "risky"} {
		c, closeFn := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"errcode":0,"result":{"suggest":"` + suggest + `","label":20001}}`))
		})
		res, err := c.CheckText(context.Background(), ports.ModerationRequest{Text: "x"})
		closeFn()
		if err != nil || res.Pass {
			t.Fatalf("%s 不应放行: %+v %v", suggest, res, err)
		}
		if len(res.Labels) == 0 || res.Labels[0] != "20001" {
			t.Fatalf("标签缺失: %+v", res)
		}
	}
}

func TestCheckTextHit(t *testing.T) {
	c, closeFn := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":87014,"errmsg":"risky content"}`))
	})
	defer closeFn()
	res, err := c.CheckText(context.Background(), ports.ModerationRequest{Text: "x"})
	if err != nil || res.Pass {
		t.Fatalf("命中应拒绝: %+v %v", res, err)
	}
}

func TestCheckTextUpstreamError(t *testing.T) {
	c, closeFn := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`bad gateway`))
	})
	defer closeFn()
	if _, err := c.CheckText(context.Background(), ports.ModerationRequest{Text: "x"}); err == nil {
		t.Fatal("上游错误应返回错误（调用方保持审核中）")
	}
}
