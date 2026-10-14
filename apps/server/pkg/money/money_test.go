package money

import "testing"

func TestYearsDiscount(t *testing.T) {
	// 年费 36500 分，3 年 0.9 折 = 36500*3*0.9 = 98550
	if got := Years(36500, 3, 9000); got != 98550 {
		t.Fatalf("got %d", got)
	}
	// 5 年 0.8 折
	if got := Years(36500, 5, 8000); got != 146000 {
		t.Fatalf("got %d", got)
	}
}

func TestDivRounding(t *testing.T) {
	// 99900 分 ÷ 3 = 33300
	if got := Fen(99900).Div(3); got != 33300 {
		t.Fatalf("got %d", got)
	}
	// 100 ÷ 3 = 33.33 → 33（四舍五入）
	if got := Fen(100).Div(3); got != 33 {
		t.Fatalf("got %d", got)
	}
}

func TestProrataRefund(t *testing.T) {
	// 三年单满一年退：2 × 36500 × 0.9 = 65700
	if got := ProrataRefund(36500, 2, 9000); got != 65700 {
		t.Fatalf("got %d", got)
	}
	if got := ProrataRefund(36500, 0, 9000); got != 0 {
		t.Fatalf("got %d", got)
	}
}
