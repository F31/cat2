// Package contract discovery 对外契约（只读投影）：卡片流、搜索、匹配解释。
package contract

import "context"

// Reason 推荐理由。
type Reason struct {
	Dimension string `json:"dimension"`
	Text      string `json:"text"`
}

// Card 卡片（广场流/搜索结果项）。
type Card struct {
	Ref         string   `json:"ref"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Cover       string   `json:"cover,omitempty"`
	Region      string   `json:"region,omitempty"`
	Score       float64  `json:"score"`
	MemberLevel int16    `json:"member_level"`
	Verified    bool     `json:"verified"`
	Reason      []Reason `json:"reason,omitempty"`
}

// SearchQuery 搜索条件（关键词与标签相互独立）。
type SearchQuery struct {
	Keyword  string   `json:"keyword"`
	Type     string   `json:"type"`
	Region   string   `json:"region"`
	Industry []string `json:"industry"`
	Need     []string `json:"need"`
	Offer    []string `json:"offer"`
	Limit    int      `json:"limit"`
}

// Explain 匹配解释。
type Explain struct {
	Ref    string   `json:"ref"`
	Score  float64  `json:"score"`
	Reason []Reason `json:"reason"`
}

// Facade discovery 对外接口（只读）。
type Facade interface {
	// Feed 匹配卡片流（读预计算候选）。
	Feed(ctx context.Context, userID int64, limit int) ([]Card, error)
	// Search 六类内容检索。
	Search(ctx context.Context, q SearchQuery) ([]Card, error)
	// Explain 某候选的匹配解释（规则模板生成）。
	Explain(ctx context.Context, userID int64, ref string) (Explain, error)
}
