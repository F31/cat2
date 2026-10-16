package ai

import (
	"context"
	"regexp"
	"strings"

	"catcube/internal/modules/ai/contract"
)

// AI 配额与成本配置键（后台可配；p5 §1.5）。
const (
	KeyQuotaPerDay     = "ai.quota.user.daily"     // 单用户每日调用上限
	KeyBudgetPerDayFen = "ai.budget.fen.daily"     // 每日成本预算（分，0=不限）
	KeyCostPer1kTokens = "ai.cost.fen_per_1ktoken" // 每千 token 成本（分）
)

// AI 观测告警阈值（p5 §8.2；0=不告警/用默认）。
const (
	KeyAlertCostFenDaily   = "ai.alert.cost.fen.daily"   // 当日成本上限（分）
	KeyAlertAdoptRateMin   = "ai.alert.adopt_rate.min"   // 采纳率下限（百分比 0-100）
	KeyAlertAdoptMinSample = "ai.alert.adopt.min_sample" // 采纳率告警最小样本量
)

// 隐私红线（p5 §1.7）：手机号/证件号/邮箱/微信号等敏感信息在构造提示词前剥离或掩码，
// 统一入口 buildPrompt，禁止模块各自拼接提示词。
var (
	phoneRe   = regexp.MustCompile(`1[3-9]\d{9}`)
	idCardRe  = regexp.MustCompile(`\d{17}[\dXx]`)
	emailRe   = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.]+`)
	wechatRe  = regexp.MustCompile(`(?i)(微信|wechat|wx|vx)\s*[:：]?\s*[A-Za-z0-9_-]{4,}`)
	sensitive = []string{"phone", "contact_phone", "id_card_no", "id_no", "real_name", "session_key", "password"}
)

// sanitizeForPrompt 剥离/掩码敏感信息（手机号、证件号、邮箱、微信号）。
func sanitizeForPrompt(text string) (string, int) {
	removed := 0
	sub := func(re *regexp.Regexp, repl string) {
		out := re.ReplaceAllString(text, repl)
		if out != text {
			removed++
			text = out
		}
	}
	sub(idCardRe, "[证件号已隐去]") // 先证件号（避免手机号规则切碎）
	sub(phoneRe, "[手机号已隐去]")
	sub(emailRe, "[邮箱已隐去]")
	sub(wechatRe, "[联系方式已隐去]")
	return text, removed
}

// buildPrompt 统一提示词构造入口（含隐私过滤）。
func buildPrompt(feature, kind, text string) (string, int) {
	clean, removed := sanitizeForPrompt(text)
	var b strings.Builder
	b.WriteString("请把下面文本整理为 ")
	b.WriteString(feature)
	if kind != "" {
		b.WriteString("（kind=" + kind + "）")
	}
	b.WriteString("：\n文本:")
	b.WriteString(clean)
	b.WriteString("\n")
	return b.String(), removed
}

// fieldAllowed 字段池白名单校验（模型只能输出已登记字段；非法丢弃并记降级，p5 §2.2/§3.2）。
func (m *Module) fieldAllowed(ctx context.Context) map[string]bool {
	if m.dict == nil {
		return nil
	}
	defs, err := m.dict.FieldDefs(ctx)
	if err != nil {
		return nil
	}
	out := make(map[string]bool, len(defs))
	for _, d := range defs {
		out[d.Field] = true
	}
	return out
}

// dropUnknownFields 丢弃未登记字段，返回保留后的字段与丢弃数。
func dropUnknownFields(fields []contract.Field, allowed map[string]bool) ([]contract.Field, int) {
	if allowed == nil {
		return fields, 0
	}
	out := make([]contract.Field, 0, len(fields))
	dropped := 0
	for _, f := range fields {
		if f.Field == "" || !allowed[f.Field] {
			dropped++
			continue
		}
		out = append(out, f)
	}
	return out, dropped
}

// filterValidCodes 仅保留字典中登记且启用的 code（类型/标签幻觉收敛）。
func (m *Module) filterValidCodes(ctx context.Context, codes []string) []string {
	if m.dict == nil || len(codes) == 0 {
		return codes
	}
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if c == "" {
			continue
		}
		o, err := m.dict.Resolve(ctx, c)
		if err == nil && o.Enabled {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
