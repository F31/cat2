// Package mask 日志与展示的敏感信息脱敏。
package mask

import (
	"regexp"
	"strings"
)

// Phone 手机号脱敏：138****5678。
func Phone(p string) string {
	if len(p) < 7 {
		return strings.Repeat("*", len(p))
	}
	return p[:3] + "****" + p[len(p)-4:]
}

// IDCard 证件号脱敏：保留前 4 后 4。
func IDCard(s string) string {
	if n := len(s); n > 8 {
		return s[:4] + strings.Repeat("*", n-8) + s[n-4:]
	}
	return strings.Repeat("*", len(s))
}

// Name 姓名脱敏：保留首字。
func Name(s string) string {
	r := []rune(s)
	if len(r) <= 1 {
		return s
	}
	return string(r[0]) + strings.Repeat("*", len(r)-1)
}

var (
	phoneLike  = regexp.MustCompile(`1[3-9]\d{9}`)
	idCardLike = regexp.MustCompile(`\d{17}[\dXx]`)
)

// ContainsPhoneLike 是否含手机号形态（导出/日志白名单校验）。
func ContainsPhoneLike(s string) bool { return phoneLike.MatchString(s) }

// ContainsIDCardLike 是否含证件号形态。
func ContainsIDCardLike(s string) bool { return idCardLike.MatchString(s) }
