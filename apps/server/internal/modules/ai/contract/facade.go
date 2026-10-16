// Package contract ai 对外契约：AI 一期场景（一句话入驻/发布草稿/自然语言搜索）。
// 全部能力受特性开关门禁；开关关闭或上游失败时返回 030001，前端回退原表单（p6 §4.1）。
package contract

import "context"

// 特性开关键（后台可配 / 灰度）。
const (
	FlagOnboard = "ai.onboard"
	FlagPublish = "ai.publish"
	FlagSearch  = "ai.search"
	FlagReview  = "ai.review" // 审核辅助（预分类/摘要，只读建议）
)

// Field 结构化字段（字段名与 dict 字段池一致）。
type Field struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// Draft 生成草稿（入驻名片 / 发布内容共用）。
type Draft struct {
	CallID int64   `json:"call_id,string"`
	Title  string  `json:"title,omitempty"`
	Fields []Field `json:"fields"`
	Tokens int     `json:"tokens"`
}

// SearchQuery 自然语言解析出的结构化检索条件。
type SearchQuery struct {
	CallID   int64    `json:"call_id,string"`
	Keyword  string   `json:"keyword,omitempty"`
	Type     string   `json:"type,omitempty"`
	Region   string   `json:"region,omitempty"`
	Industry []string `json:"industry,omitempty"`
	Need     []string `json:"need,omitempty"`
	Offer    []string `json:"offer,omitempty"`
	Tokens   int      `json:"tokens"`
}

// Suggestion 审核辅助建议（只读，展示给审核员；不自动裁决）。
type Suggestion struct {
	CallID   int64  `json:"call_id,string"`
	Category string `json:"category"` // 预分类：fraud/ad/abuse/porn/other
	Risk     int    `json:"risk"`     // 0-3（0 低）
	Summary  string `json:"summary"`  // 摘要
	Tokens   int    `json:"tokens"`
}

// Prompt 提示词版本（仅 approved=true 的最高版本生效）。
type Prompt struct {
	Feature  string `json:"feature"`
	Version  string `json:"version"`
	Body     string `json:"body"`
	Approved bool   `json:"approved"`
}

// Usage 能力用量与采纳/成本统计（后台看板，p5 §7.2/§8.2）。
type Usage struct {
	Feature   string  `json:"feature"`
	Calls     int64   `json:"calls"`      // outcome=ok 的调用数
	Degraded  int64   `json:"degraded"`   // 降级次数
	Adopted   int64   `json:"adopted"`    // 采纳数
	Rejected  int64   `json:"rejected"`   // 放弃数
	AdoptRate float64 `json:"adopt_rate"` // 采纳率（adopted / 有反馈的调用）
	Tokens    int64   `json:"tokens"`
	CostFen   int64   `json:"cost_fen"` // 估算成本（分）
}

// Facade ai 对外接口（供 bff/其他模块复用）。
type Facade interface {
	// OnboardDraft 一句话入驻 → 名片草稿（开关 ai.onboard）。
	OnboardDraft(ctx context.Context, userID int64, text string) (Draft, error)
	// PublishDraft 一句话发布 → 内容草稿（开关 ai.publish）。
	PublishDraft(ctx context.Context, userID int64, kind, text string) (Draft, error)
	// Search 自然语言搜索 → 结构化条件（开关 ai.search）。
	Search(ctx context.Context, userID int64, text string) (SearchQuery, error)
	// Feedback 采纳反馈（采纳率观测）。
	Feedback(ctx context.Context, userID, callID int64, adopted bool) error
	// ReviewSuggestion 审核辅助建议（开关 ai.review；不可用返回 030001，调用方按「无建议」处理）。
	ReviewSuggestion(ctx context.Context, operatorID int64, subjectType string, subjectID int64, text string) (Suggestion, error)
	// Enabled 能力是否开启（按用户灰度评估）。
	Enabled(ctx context.Context, userID int64, feature string) bool

	// Prompts 提示词版本列表（后台管理，p5 §7.2）。
	Prompts(ctx context.Context) ([]Prompt, error)
	// SavePrompt 新增/更新提示词版本（approved 决定是否生效；变更须过评测）。
	SavePrompt(ctx context.Context, feature, version, body string, approved bool) error
	// Usage 近 days 天各能力用量/采纳率/成本（后台看板 + 告警，p5 §7.2/§8.2）。
	Usage(ctx context.Context, days int) ([]Usage, error)
}
