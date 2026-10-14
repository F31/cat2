package clock

import (
	"testing"
	"time"
)

func TestNaturalDay(t *testing.T) {
	// UTC 2026-10-13 16:30 = 上海 2026-10-14 00:30 → 自然日应为 10-14
	ts := time.Date(2026, 10, 13, 16, 30, 0, 0, time.UTC)
	day := NaturalDay(ts)
	if got := day.In(shanghai).Format("2006-01-02"); got != "2026-10-14" {
		t.Fatalf("got %s", got)
	}
	// UTC 2026-10-13 15:59 = 上海 23:59 → 自然日 10-13
	ts2 := time.Date(2026, 10, 13, 15, 59, 0, 0, time.UTC)
	if got := NaturalDay(ts2).In(shanghai).Format("2006-01-02"); got != "2026-10-13" {
		t.Fatalf("got %s", got)
	}
}

func TestAddNaturalDays(t *testing.T) {
	day := NaturalDay(time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC))
	next := AddNaturalDays(day, 30)
	if got := next.In(shanghai).Format("2006-01-02"); got != "2026-11-12" {
		t.Fatalf("got %s", got)
	}
}
