// Package contract trust 对外契约：内容安全审核、举报、限制、审核队列。
package contract

import "context"

// 审核任务来源。
const (
	SourceMachine  int16 = 1 // 机器命中
	SourceReport   int16 = 2 // 举报
	SourceCert     int16 = 3 // 认证材料
	SourceProduct  int16 = 4 // 产品入库
	SourceHighRisk int16 = 5 // 高风险类目抽检
)

// ReviewTask 审核任务（统一队列）。
type ReviewTask struct {
	ID          int64  `json:"id,string"`
	Source      int16  `json:"source"`
	SubjectType string `json:"subject_type"`
	SubjectID   int64  `json:"subject_id,string"`
	RiskLevel   int16  `json:"risk_level"`
	Status      int16  `json:"status"` // 0待审/1通过/2驳回/3已关闭
	Reason      string `json:"reason,omitempty"`
	SLAOverdue  bool   `json:"sla_overdue"`
}

// Verdict 复核结论。
type Verdict struct {
	TaskID      int64  `json:"task_id,string"`
	SubjectType string `json:"subject_type"`
	SubjectID   int64  `json:"subject_id,string"`
	UserID      int64  `json:"user_id,string"`
	Points      int    `json:"points"`
	Total       int    `json:"total"`
	Restricted  bool   `json:"restricted"`
}

// SLAMetric 审核队列指标。
type SLAMetric struct {
	Pending          int `json:"pending"`
	Overdue          int `json:"overdue"`
	AvgReviewSeconds int `json:"avg_review_seconds"`
}

// Facade trust 对外接口。
type Facade interface {
	// SubmitReview 提交审核（敏感词前置 + 文本同步机检）；结论经事件对外。
	SubmitReview(ctx context.Context, subjectType string, subjectID int64, text string, imageIDs []string) (pass bool, err error)
	// Report 用户举报（入人工复核队列，24h SLA）。
	Report(ctx context.Context, reporterID int64, targetRef, reasonCode, detail string) error
	// Restrict 限制/封禁。
	Restrict(ctx context.Context, userID int64, days int, reason string) error
	// IsRestricted 是否被限制发布（能力提供者）。
	IsRestricted(ctx context.Context, userID int64) (bool, error)
	// ResolveReview 后台复核结论（confirmed=true 属实：驳回对象 + 记分 + 必要时限制）。
	ResolveReview(ctx context.Context, taskID, reviewerID int64, confirmed bool, reason string) (Verdict, error)
	// ReviewQueue 待审队列（按 SLA 到期排序）。
	ReviewQueue(ctx context.Context, limit int) ([]ReviewTask, error)
	// ViolationPoints 累计违规分。
	ViolationPoints(ctx context.Context, userID int64) (int, error)
	// SLAMetrics 队列指标。
	SLAMetrics(ctx context.Context) (SLAMetric, error)
}

// ReviewSubjectProvider 审核对象提供者开口（扩展点 #1）：
// 由 content/msg/circle/profile/verify 注册「怎么取文字/图片、归属人是谁、结论怎么落回」。
type ReviewSubjectProvider interface {
	SubjectType() string
	// Extract 取审核素材（文字/图片）。
	Extract(ctx context.Context, subjectID int64) (text string, imageIDs []string, err error)
	// OwnerOf 归属人（用于违规记分/限制对象）。
	OwnerOf(ctx context.Context, subjectID int64) (userID int64, err error)
	// ApplyVerdict 审核结论落回（pass=true 通过）。
	ApplyVerdict(ctx context.Context, subjectID int64, pass bool, reason string) error
}

// ReportReason 举报原因枚举（机器可读 code）。
type ReportReason struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// ReportReasons 举报原因清单。
var ReportReasons = []ReportReason{
	{Code: "spam", Label: "垃圾广告"},
	{Code: "fraud", Label: "诈骗信息"},
	{Code: "porn", Label: "色情低俗"},
	{Code: "abuse", Label: "辱骂骚扰"},
	{Code: "infringe", Label: "侵权盗用"},
	{Code: "other", Label: "其他"},
}

// ValidReportReason 校验举报原因。
func ValidReportReason(code string) bool {
	for _, r := range ReportReasons {
		if r.Code == code {
			return true
		}
	}
	return false
}
