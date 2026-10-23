package wechat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendSubscribeMessage(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cgi-bin/message/subscribe/send" || r.URL.Query().Get("access_token") != "tk" {
			t.Fatalf("请求错误: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()
	c := NewNotifyClient(func(context.Context) (string, error) { return "tk", nil })
	c.BaseURL = srv.URL
	if err := c.SendSubscribeMessage(context.Background(), "openid-1", "tmpl-1", map[string]string{"thing1": "标题"}); err != nil {
		t.Fatal(err)
	}
	if body["touser"] != "openid-1" || body["template_id"] != "tmpl-1" {
		t.Fatalf("请求体错误: %+v", body)
	}
}
