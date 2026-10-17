// Package contract channel 对外契约：运营商身份、归属关系、佣金与业绩（p4 §1）。
package contract

import "context"

// 运营商状态：0 申请中 / 1 已开通 / 2 已停用。
const (
	StatusApplying  int16 = 0
	StatusOpened    int16 = 1
	StatusSuspended int16 = 2
)

// Operator 运营商。
type Operator struct {
	ID                 int64  `json:"id,string"`
	UserID             int64  `json:"user_id,string"`
	RegionCode         string `json:"region_code"`
	RegionName         string `json:"region_name,omitempty"`
	Name               string `json:"name"`
	Status             int16  `json:"status"`
	ReferrerOperatorID int64  `json:"referrer_operator_id,string"` // 单层展示，无上级链（红线）
	OpenedAt           int64  `json:"opened_at,omitempty"`
}

// Attribution 归属关系（有效归属同时仅一条）。
type Attribution struct {
	ID         int64  `json:"id,string"`
	UserID     int64  `json:"user_id,string"`
	OperatorID int64  `json:"operator_id,string"`
	RegionCode string `json:"region_code"`
	Source     int16  `json:"source"` // 1 链接 / 2 二维码 / 3 邀请码
	BoundAt    int64  `json:"bound_at"`
}

// Commission 佣金明细。
type Commission struct {
	ID        int64  `json:"id,string"`
	OrderID   int64  `json:"order_id,string"`
	BizType   string `json:"biz_type"`
	BaseFen   int64  `json:"base_fen"`
	RatioBp   int    `json:"ratio_bp"`
	AmountFen int64  `json:"amount_fen"`
	Month     string `json:"month"`
	Status    int16  `json:"status"`
}

// Target 保底任务进度（12 个月 ≥50 付费会员 或 ≥3 落地项目）。
type Target struct {
	Year                int `json:"year"`
	PaidMemberCount     int `json:"paid_member_count"`
	LandingProjectCount int `json:"landing_project_count"`
}

// Overview 业绩概览。
type Overview struct {
	Operator    Operator     `json:"operator"`
	Month       string       `json:"month"`
	MonthFen    int64        `json:"month_fen"` // 本月佣金合计（分）
	TotalFen    int64        `json:"total_fen"` // 累计佣金（分）
	Target      Target       `json:"target"`
	Commissions []Commission `json:"commissions"`
}

// Facade channel 对外接口（供 bff/其他模块复用）。
type Facade interface {
	// OperatorOf 用户是否为运营商（移动端身份卡/后台）。
	OperatorOf(ctx context.Context, userID int64) (Operator, bool, error)
	// Apply 申请/重新申请运营商（区域查重一地一家；referrer 单层展示）。
	Apply(ctx context.Context, userID int64, regionCode, name string, referrerOperatorID int64) (Operator, error)
	// OpenOperator 开通（后台审核；0→1）。
	OpenOperator(ctx context.Context, operatorID int64) error
	// SuspendOperator 停用（1→2）。
	SuspendOperator(ctx context.Context, operatorID int64) error
	// AttributionOf 用户的有效归属。
	AttributionOf(ctx context.Context, userID int64) (Attribution, bool, error)
	// BindAttribution 绑定归属（按推广码/邀请码；幂等，单一有效）。
	BindAttribution(ctx context.Context, userID int64, code string) (Attribution, error)
	// Overview 业绩概览（佣金合计 + 保底任务）。
	Overview(ctx context.Context, userID int64, month string) (Overview, error)
	// ListOperators 后台运营商列表（可按状态/区域过滤）。
	ListOperators(ctx context.Context, status int16, regionCode string, limit int) ([]Operator, error)
}
