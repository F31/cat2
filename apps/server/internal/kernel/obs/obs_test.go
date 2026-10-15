package obs

import (
	"bytes"
	"log/slog"
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
