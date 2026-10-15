package phone

import "testing"

func TestValid(t *testing.T) {
	for _, s := range []string{"13812345678", "19900000000", "15012345678"} {
		if !ValidMobile(s) {
			t.Fatalf("合法手机号误判: %s", s)
		}
	}
	for _, s := range []string{"12345678901", "1381234567", "23812345678", ""} {
		if ValidMobile(s) {
			t.Fatalf("非法手机号误判: %s", s)
		}
	}
	if !ValidIDCard("11010119900307001X") || !ValidIDCard("110101199003070019") {
		t.Fatal("合法身份证误判")
	}
	for _, s := range []string{"11010119900307001", "1101011990030700X9", "abcdefghijklmnopqr", ""} {
		if ValidIDCard(s) {
			t.Fatalf("非法身份证误判: %s", s)
		}
	}
}
