package domain

import (
	"strings"
	"testing"
)

func TestDMKey(t *testing.T) {
	if min, max := DMKey(9, 3); min != 3 || max != 9 {
		t.Fatalf("DMKey(9,3)=%d,%d", min, max)
	}
	if min, max := DMKey(3, 9); min != 3 || max != 9 {
		t.Fatalf("DMKey(3,9)=%d,%d", min, max)
	}
	if min, max := DMKey(5, 5); min != 5 || max != 5 {
		t.Fatalf("DMKey(5,5)=%d,%d", min, max)
	}
}

func TestValidateContent(t *testing.T) {
	if err := ValidateContent(1, "你好"); err != nil {
		t.Fatalf("合法文本被拒: %v", err)
	}
	if err := ValidateContent(1, "   "); err != ErrEmptyContent {
		t.Fatalf("空白应为 ErrEmptyContent: %v", err)
	}
	if err := ValidateContent(1, strings.Repeat("字", MaxTextLen+1)); err != ErrContentTooLong {
		t.Fatalf("超长应为 ErrContentTooLong: %v", err)
	}
	if err := ValidateContent(9, "x"); err != ErrBadMsgType {
		t.Fatalf("非法类型应为 ErrBadMsgType: %v", err)
	}
	if err := ValidateContent(2, "file-123"); err != nil {
		t.Fatalf("图片消息不校验长度: %v", err)
	}
}

func TestClampLimit(t *testing.T) {
	if got := ClampLimit(0); got != DefaultPage {
		t.Fatalf("默认分页应 %d，得 %d", DefaultPage, got)
	}
	if got := ClampLimit(1000); got != MaxPage {
		t.Fatalf("超限应 %d，得 %d", MaxPage, got)
	}
	if got := ClampLimit(10); got != 10 {
		t.Fatalf("范围内应原样返回，得 %d", got)
	}
}
