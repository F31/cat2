// Package contract trust 对外契约：内容安全审核、举报、限制。
package contract

import "context"

// ReviewTask 审核任务（统一队列：机器命中/举报/认证材料/产品入库/高风险抽检）。
type ReviewTask struct {
	ID          int64  `json:"id,string"`
	Source      int16  `json:"source"` // 1机器/2举报/3认证材料/4产品入库/5高风险抽检
	SubjectType string `json:"subject_type"`
	SubjectID   int64  `json:"subject_id,string"`
	RiskLevel   int16  `json:"risk_level"`
	Status      int16  `json:"status"` // 0待审/1通过/2驳回/3已关闭
}

// Facade trust 对外接口。
type Facade interface {
	// SubmitReview 提交审核（文本同步机检；图片异步）。
	SubmitReview(ctx context.Context, subjectType string, subjectID int64, text string, imageIDs []string) (pass bool, err error)
	// Report 用户举报。
	Report(ctx context.Context, reporterID int64, targetRef, reasonCode, detail string) error
	// Restrict 限制/封禁。
	Restrict(ctx context.Context, userID int64, days int, reason string) error
	// IsRestricted 是否被限制发布（能力提供者）。
	IsRestricted(ctx context.Context, userID int64) (bool, error)
}

// ReviewSubjectProvider 审核对象提供者开口（扩展点 #1）：
// 由 content/msg/circle/profile/verify 注册「怎么取文字/图片、结论怎么落回」。
type ReviewSubjectProvider interface {
	SubjectType() string
	// Extract 取审核素材（文字/图片）。
	Extract(ctx context.Context, subjectID int64) (text string, imageIDs []string, err error)
	// ApplyVerdict 审核结论落回（pass=true 通过）。
	ApplyVerdict(ctx context.Context, subjectID int64, pass bool, reason string) error
}
