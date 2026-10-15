package mask

import "testing"

func TestMask(t *testing.T) {
	if got := Phone("13812345678"); got != "138****5678" {
		t.Fatalf("手机号脱敏: %s", got)
	}
	if got := Phone("123"); got != "***" {
		t.Fatalf("短号应全掩码: %s", got)
	}
	if got := IDCard("110101199003070019"); got != "1101**********0019" {
		t.Fatalf("证件脱敏: %s", got)
	}
	if got := IDCard("12345678"); got != "********" {
		t.Fatalf("短证件全掩码: %s", got)
	}
	if got := Name("张三丰"); got != "张**" {
		t.Fatalf("姓名脱敏: %s", got)
	}
	if got := Name("张"); got != "张" {
		t.Fatalf("单字姓名不掩码: %s", got)
	}
}
