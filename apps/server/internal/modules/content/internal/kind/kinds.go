package kind

import (
	"fmt"
	"strconv"
	"strings"
)

// ---- project（项目，8 字段）----

type projectKind struct{}

func (projectKind) Name() string { return "project" }
func (projectKind) Fields() []FieldDef {
	return []FieldDef{
		{Key: "amount_wan", Type: TypeDecimal, Required: true},
		{Key: "stage", Type: TypeText},
		{Key: "core_strength", Type: TypeText, MaxLen: 500, Member: true},
		{Key: "key_points", Type: TypeText, MaxLen: 500},
		{Key: "intro", Type: TypeText, MaxLen: 2000},
		{Key: "target_customers", Type: TypeText},
		{Key: "target_channels", Type: TypeText},
		{Key: "contact_phone", Type: TypePhone, Member: true},
	}
}
func (projectKind) CardFields() []string { return []string{"amount_wan", "stage"} }
func (projectKind) DetailFields() []string {
	return []string{"core_strength", "key_points", "intro", "target_customers", "target_channels"}
}
func (projectKind) MemberFields() []string { return []string{"core_strength", "contact_phone"} }
func (k projectKind) ReviewText(title string, f map[string]any) string {
	return joinText(title, textOf(f, "core_strength"), textOf(f, "key_points"), textOf(f, "intro"), textOf(f, "target_customers"), textOf(f, "target_channels"))
}
func (k projectKind) ReviewImages(cover int64, _ map[string]any) []string { return coverImages(cover) }

// ---- product（产品，字段独立于项目）----

type productKind struct{}

func (productKind) Name() string { return "product" }
func (productKind) Fields() []FieldDef {
	return []FieldDef{
		{Key: "carousel_file_ids", Type: TypeCodes},
		{Key: "target_customers", Type: TypeText},
		{Key: "target_channels", Type: TypeText},
		{Key: "core_strength", Type: TypeText, MaxLen: 500, Member: true},
		{Key: "key_points", Type: TypeText, MaxLen: 500},
		{Key: "intro", Type: TypeText, MaxLen: 2000},
		{Key: "contact_phone", Type: TypePhone, Member: true},
		{Key: "warehouse_status", Type: TypeInt},
	}
}
func (productKind) CardFields() []string { return []string{"carousel_file_ids", "warehouse_status"} }
func (productKind) DetailFields() []string {
	return []string{"core_strength", "key_points", "intro", "target_customers", "target_channels"}
}
func (productKind) MemberFields() []string { return []string{"core_strength", "contact_phone"} }
func (k productKind) ReviewText(title string, f map[string]any) string {
	return joinText(title, textOf(f, "core_strength"), textOf(f, "key_points"), textOf(f, "intro"), textOf(f, "target_customers"), textOf(f, "target_channels"))
}
func (k productKind) ReviewImages(cover int64, f map[string]any) []string {
	out := coverImages(cover)
	for _, c := range codesOf(f, "carousel_file_ids") {
		out = append(out, c)
	}
	return out
}

// ---- meeting（会议，不含目标客户/渠道）----

type meetingKind struct{}

func (meetingKind) Name() string { return "meeting" }
func (meetingKind) Fields() []FieldDef {
	return []FieldDef{
		{Key: "start_at", Type: TypeTime, Required: true},
		{Key: "end_at", Type: TypeTime, Required: true},
		{Key: "location", Type: TypeText, Required: true},
		{Key: "venue_point", Type: TypeText},
		{Key: "participant_limit", Type: TypeInt},
		{Key: "signup_method", Type: TypeText},
		{Key: "intro", Type: TypeText, MaxLen: 2000},
	}
}
func (meetingKind) CardFields() []string { return []string{"start_at", "location"} }
func (meetingKind) DetailFields() []string {
	return []string{"intro", "venue_point", "signup_method", "participant_limit"}
}
func (meetingKind) MemberFields() []string { return nil }
func (k meetingKind) ReviewText(title string, f map[string]any) string {
	return joinText(title, textOf(f, "location"), textOf(f, "intro"))
}
func (k meetingKind) ReviewImages(cover int64, _ map[string]any) []string { return coverImages(cover) }

// ---- event（活动，状态按时间计算）----

type eventKind struct{}

func (eventKind) Name() string { return "event" }
func (eventKind) Fields() []FieldDef {
	return []FieldDef{
		{Key: "poster_file_id", Type: TypeInt},
		{Key: "start_at", Type: TypeTime, Required: true},
		{Key: "end_at", Type: TypeTime, Required: true},
		{Key: "location", Type: TypeText, Required: true},
		{Key: "participant_limit", Type: TypeInt},
		{Key: "participant_count", Type: TypeInt},
		{Key: "signup_method", Type: TypeText},
		{Key: "intro", Type: TypeText, MaxLen: 2000},
	}
}
func (eventKind) CardFields() []string { return []string{"start_at", "location", "participant_count"} }
func (eventKind) DetailFields() []string {
	return []string{"intro", "signup_method", "participant_limit", "participant_count"}
}
func (eventKind) MemberFields() []string { return nil }
func (k eventKind) ReviewText(title string, f map[string]any) string {
	return joinText(title, textOf(f, "location"), textOf(f, "intro"))
}
func (k eventKind) ReviewImages(cover int64, f map[string]any) []string {
	out := coverImages(cover)
	if v, ok := numOf(f, "poster_file_id"); ok && v != 0 {
		out = append(out, strconv.FormatInt(v, 10))
	}
	return out
}

// ---- 公共辅助 ----

func joinText(parts ...string) string {
	var b []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			b = append(b, strings.TrimSpace(p))
		}
	}
	return strings.Join(b, "\n")
}

func coverImages(cover int64) []string {
	if cover == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%d", cover)}
}

func numOf(f map[string]any, key string) (int64, bool) {
	switch t := f[key].(type) {
	case int:
		return int64(t), true
	case int64:
		return t, true
	case float64:
		return int64(t), true
	}
	return 0, false
}
