// Package domain notice 领域：渠道、合并规则与模板渲染（无 I/O）。
package domain

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// 渠道。
const (
	ChannelInApp     = "inapp"
	ChannelSubscribe = "subscribe"
	ChannelSMS       = "sms"
)

// 合并规则。
const (
	MergeSameCircleDay = "same_circle_day"
)

// Template 通知模板。
type Template struct {
	Code         string
	Module       string
	EventCode    string
	TitleTpl     string
	ContentTpl   string
	Channels     []string
	ReceiverRule string // payload.<field>；payload.<field> 不含则跳过
	MergeRule    string
	Closeable    bool
}

var (
	ErrTemplateNotFound = errors.New("notice: 模板不存在或未启用")
	ErrNotFound         = errors.New("notice: 通知不存在")
)

// Render 文案渲染（{field} 取 payload 顶层字段）。
func Render(tpl string, payload map[string]any) string {
	out := tpl
	for {
		start := strings.Index(out, "{")
		if start < 0 {
			break
		}
		end := strings.Index(out[start:], "}")
		if end < 0 {
			break
		}
		end += start
		key := out[start+1 : end]
		val := ""
		if v, ok := payload[key]; ok {
			switch t := v.(type) {
			case string:
				val = t
			case float64:
				val = strconv.FormatFloat(t, 'f', -1, 64)
			default:
				if raw, err := json.Marshal(v); err == nil {
					val = strings.Trim(string(raw), `"`)
				}
			}
		}
		out = out[:start] + val + out[end+1:]
	}
	return out
}

// Receiver 从 payload 解析接收人（receiver_rule 形如 payload.user_id 或 payload.owner_ref.id）。
func Receiver(rule string, payload map[string]any) (int64, bool) {
	path := strings.TrimPrefix(rule, "payload.")
	var cur any = payload
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0, false
		}
		cur, ok = m[seg]
		if !ok {
			return 0, false
		}
	}
	switch t := cur.(type) {
	case string:
		var id int64
		for _, c := range t {
			if c < '0' || c > '9' {
				return 0, false
			}
			id = id*10 + int64(c-'0')
		}
		return id, id > 0
	case float64:
		return int64(t), t > 0
	}
	return 0, false
}
