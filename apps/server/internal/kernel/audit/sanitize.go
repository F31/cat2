package audit

import (
	"regexp"
	"strings"

	"catcube/pkg/mask"
)

// 敏感字段名单（写入审计 detail 前统一脱敏；p4 §5.2）。
var sensitiveKeys = map[string]bool{
	"phone": true, "contact_phone": true, "mobile": true, "phone_enc": true,
	"id_card_no": true, "id_no": true, "id_card": true,
	"real_name": true, "id_card_name": true,
	"session_key": true, "access_token": true, "refresh_token": true, "token": true,
	"password": true, "secret": true, "appsecret": true, "app_secret": true,
	"encrypted_data": true, "iv": true, "private_key": true, "enc_key": true,
	"bank_card": true, "openid": true, "wechat_openid": true,
}

var (
	auditPhoneRe  = regexp.MustCompile(`1[3-9]\d{9}`)
	auditIDCardRe = regexp.MustCompile(`\d{17}[\dXx]`)
)

// SanitizeDetail 递归脱敏 detail 中的敏感字段（键名命中即掩码；值内嵌手机号/证件号一并抹除）。
func SanitizeDetail(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if sensitiveKeys[strings.ToLower(k)] {
				out[k] = maskField(k, val)
				continue
			}
			out[k] = SanitizeDetail(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = SanitizeDetail(x)
		}
		return out
	case string:
		return scrubString(t)
	}
	return v
}

func maskField(key string, val any) any {
	s, ok := val.(string)
	if !ok {
		return "[redacted]"
	}
	switch strings.ToLower(key) {
	case "phone", "contact_phone", "mobile":
		return mask.Phone(s)
	case "id_card_no", "id_no", "id_card":
		return mask.IDCard(s)
	case "real_name", "id_card_name":
		return mask.Name(s)
	default:
		return "[redacted]"
	}
}

func scrubString(s string) string {
	s = auditIDCardRe.ReplaceAllStringFunc(s, func(m string) string { return mask.IDCard(m) })
	s = auditPhoneRe.ReplaceAllStringFunc(s, func(m string) string { return mask.Phone(m) })
	return s
}
