// Package phone 手机号与证件号格式校验。
package phone

import "regexp"

var (
	mobileRe  = regexp.MustCompile(`^1[3-9]\d{9}$`)
	idCardRe  = regexp.MustCompile(`^\d{17}[\dXx]$`)
)

// ValidMobile 中国大陆手机号格式（不做号段以外的可用性判断）。
func ValidMobile(s string) bool { return mobileRe.MatchString(s) }

// ValidIDCard 18 位身份证号格式（不校验校验位之外的语义）。
func ValidIDCard(s string) bool { return idCardRe.MatchString(s) }
