package obs

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 脱敏处理器：密钥类屏蔽；手机号/证件号/姓名掩码；普通字段原样。
func TestRedactingHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buf, nil)))
	logger.Info("login",
		"session_key", "s3cr3t-session",
		"phone", "13812345678",
		"id_card_no", "110101199003070019",
		"real_name", "张三丰",
		"user_id", 1001,
		"note", "联系 13900000000 客户",
		"access_token", "tok-abc",
	)
	out := buf.String()
	for _, leak := range []string{"s3cr3t-session", "13812345678", "110101199003070019", "张三丰", "tok-abc"} {
		if strings.Contains(out, leak) {
			t.Fatalf("日志泄漏敏感信息 %q: %s", leak, out)
		}
	}
	if !strings.Contains(out, "138****5678") || !strings.Contains(out, "1101**********0019") {
		t.Fatalf("应掩码输出: %s", out)
	}
	if !strings.Contains(out, `"user_id":1001`) {
		t.Fatalf("普通字段应保留: %s", out)
	}
	if !strings.Contains(out, "139****0000") {
		t.Fatalf("值内手机号应掩码: %s", out)
	}
}

func TestWithUserIDAndModule(t *testing.T) {
	ctx := WithUserID(WithModule(WithRequestID(t.Context(), "rid-1"), "identity"), 42)
	if v, _ := ctx.Value(CtxUserID).(int64); v != 42 {
		t.Fatal("user_id 未注入")
	}
}

func TestParseTraceparentAndMiddleware(t *testing.T) {
	tid, sid := parseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if tid != "4bf92f3577b34da6a3ce929d0e0e4736" || sid != "00f067aa0ba902b7" {
		t.Fatalf("解析错误: %q %q", tid, sid)
	}
	if _, _ = parseTraceparent("bad"); false {
		t.Fatal("unreachable")
	}
	if tid, _ := parseTraceparent("bad"); tid != "" {
		t.Fatal("非法 traceparent 应返回空")
	}
	// 中间件：透传 traceparent 并注入 ctx
	var got string
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = TraceID(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("ctx trace 透传失败: %q", got)
	}
	if tp := rec.Header().Get("Traceparent"); tp == "" {
		t.Fatal("响应应回写 Traceparent")
	}
	// 无入参：自动生成 32hex
	req2 := httptest.NewRequest(http.MethodGet, "/y", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if got == "" || len(got) != 32 {
		t.Fatalf("应自动生成 trace id: %q", got)
	}
}
