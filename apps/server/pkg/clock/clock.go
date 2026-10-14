// Package clock 时间约定：库内 timestamptz(UTC)；业务「自然日」统一 Asia/Shanghai。
package clock

import "time"

var shanghai = time.FixedZone("Asia/Shanghai", 8*3600)

// NowUTC 当前 UTC 时间。
func NowUTC() time.Time { return time.Now().UTC() }

// Shanghai 时区。
func Shanghai() *time.Location { return shanghai }

// NaturalDay 某时刻对应的自然日（Asia/Shanghai，返回当日 0 点的 UTC 时间）。
func NaturalDay(t time.Time) time.Time {
	local := t.In(shanghai)
	y, m, d := local.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, shanghai).UTC()
}

// Today 当前自然日（Asia/Shanghai）。
func Today() time.Time { return NaturalDay(time.Now()) }

// AddNaturalDays 自然日加减。
func AddNaturalDays(day time.Time, n int) time.Time {
	return day.In(shanghai).AddDate(0, 0, n).UTC()
}
