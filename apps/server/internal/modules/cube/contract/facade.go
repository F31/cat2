// Package contract cube 对外契约：猫立方摘要、角色、项目引用与协作用例。
package contract

import (
	"context"
	"time"
)

// 成员角色。
const (
	RoleNone     int16 = 0
	RoleLeader   int16 = 1 // 猫王
	RoleHoney    int16 = 2 // 猫蜜
	RoleMember   int16 = 3 // 普通成员
	RoleCoBuild  int16 = 4 // 共建成员（分摊成团）
)

// 立方状态（状态机 #5）。
const (
	CubeActive    int16 = 0
	CubeBanned    int16 = 1
	CubeDissolved int16 = 2
)

// 成员状态（状态机 #6）。
const (
	MemberActive  int16 = 0
	MemberLeft    int16 = 1
	MemberRemoved int16 = 2
)

// 订阅档位。
const (
	PlanBasic   int16 = 1
	PlanPlus    int16 = 2
	PlanPremium int16 = 3
)

// CubeBrief 立方摘要（EntityResolver/展示用）。
type CubeBrief struct {
	ID          int64  `json:"id,string"`
	Name        string `json:"name"`
	LeaderID    int64  `json:"leader_id,string"`
	HoneyID     int64  `json:"honey_id,string"`
	PlanLevel   int16  `json:"plan_level"`
	PlanExpireAt *time.Time `json:"plan_expire_at,omitempty"`
	Status      int16  `json:"status"`
	MemberCount int    `json:"member_count"`
	RegionCode  string `json:"region_code,omitempty"`
}

// CubeDetail 立方详情（含画像字段）。
type CubeDetail struct {
	CubeBrief
	OrgID        int64    `json:"org_id,string,omitempty"`
	IndustryCodes []string `json:"industry_codes"`
	ChainStage   string   `json:"chain_stage"`
	Direction    string   `json:"direction"`
	Resources    string   `json:"resources"`
	Strength     string   `json:"strength"`
	Cooperation  string   `json:"cooperation"`
	MyRole       int16    `json:"my_role"`
}

// Member 成员。
type Member struct {
	UserID   int64     `json:"user_id,string"`
	Role     int16     `json:"role"`
	Status   int16     `json:"status"`
	JoinedAt time.Time `json:"joined_at"`
}

// CreateInput 创建入参。
type CreateInput struct {
	Name          string
	OrgID         int64
	RegionCode    string
	IndustryCodes []string
	ChainStage    string
	Direction     string
	Resources     string
	Strength      string
	Cooperation   string
	HoneyUserID   int64 // 必配猫蜜（不可为自己）
}

// InviteTicket 邀请凭证（邀请码；前端据此生成卡片/二维码）。
type InviteTicket struct {
	Code        string    `json:"code"`
	ExpireAt    time.Time `json:"expire_at"`
	NeedConfirm bool      `json:"need_confirm"`
}

// Project 立方内项目（引用 + 本地状态）。
type Project struct {
	ID              int64     `json:"id,string"`
	CubeID          int64     `json:"cube_id,string"`
	Type            int16     `json:"type"` // 1研究/2承接
	ContentProjectID int64    `json:"content_project_id,string,omitempty"`
	Name            string    `json:"name"`
	RelatedRef      string    `json:"related_ref,omitempty"`
	RelatedCustomer string    `json:"related_customer,omitempty"`
	LeaderID        int64     `json:"leader_id,string"`
	Participants    []string  `json:"participants"`
	Progress        int16     `json:"progress"`
	TodoCount       int       `json:"todo_count"`
	Status          int16     `json:"status"` // 0进行中/1已关闭
	SourceRef       string    `json:"source_ref,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// Task 任务（四态；project_id=0 表示内部工作安排）。
type Task struct {
	ID            int64      `json:"id,string"`
	CubeID        int64      `json:"cube_id,string"`
	ProjectID     int64      `json:"project_id,string,omitempty"`
	Title         string     `json:"title"`
	AssigneeID    int64      `json:"assignee_id,string"`
	Progress      int16      `json:"progress"`
	ConfirmUserID int64      `json:"confirm_user_id,string,omitempty"`
	DueAt         *time.Time `json:"due_at,omitempty"`
	Status        int16      `json:"status"` // 0待办/1进行中/2待确认/3已完成
	RejectReason  string     `json:"reject_reason,omitempty"`
	ReopenedCount int        `json:"reopened_count"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Report 重点客户报备。
type Report struct {
	ID             int64      `json:"id,string"`
	CubeID         int64      `json:"cube_id,string"`
	ProjectID      int64      `json:"project_id,string,omitempty"`
	ReporterID     int64      `json:"reporter_id,string"`
	CustomerName   string     `json:"customer_name"`
	CustomerType   string     `json:"customer_type,omitempty"`
	RegionCode     string     `json:"region_code,omitempty"`
	RelatedItemRef string     `json:"related_item_ref,omitempty"`
	AmountWan      int64      `json:"amount_wan,omitempty"`
	Note           string     `json:"note,omitempty"`
	Status         int16      `json:"status"` // 0待确认/1已确认/2已驳回
	RejectReason   string     `json:"reject_reason,omitempty"`
	ConfirmedAt    *time.Time `json:"confirmed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Research 研究记录。
type Research struct {
	ID         int64     `json:"id,string"`
	ProjectID  int64     `json:"project_id,string"`
	Content    string    `json:"content"`
	Decision   string    `json:"decision,omitempty"`
	OperatorID int64     `json:"operator_id,string"`
	CreatedAt  time.Time `json:"created_at"`
}

// ProjectInput 新建项目入参。
type ProjectInput struct {
	Type            int16
	Name            string
	RelatedRef      string
	RelatedCustomer string
	Participants    []string
	ContentProjectID int64
	SourceRef       string
}

// TaskInput 新建任务入参。
type TaskInput struct {
	ProjectID     int64
	Title         string
	AssigneeID    int64
	ConfirmUserID int64
	DueAt         *time.Time
}

// ReportInput 报备入参。
type ReportInput struct {
	ProjectID      int64
	CustomerName   string
	ContactPhone   string
	CustomerType   string
	RegionCode     string
	RelatedItemRef string
	AmountWan      int64
	Note           string
}

// Facade cube 对外接口。
type Facade interface {
	Brief(ctx context.Context, cubeID int64) (CubeBrief, error)
	RoleOf(ctx context.Context, cubeID, userID int64) (int16, error)
	ListMine(ctx context.Context, userID int64) ([]CubeBrief, error)
	Detail(ctx context.Context, viewerID, cubeID int64) (CubeDetail, error)
	Create(ctx context.Context, userID int64, in CreateInput) (CubeBrief, error)
	Invite(ctx context.Context, cubeID, byUserID int64) (InviteTicket, error)
	JoinByCode(ctx context.Context, userID int64, code string) (CubeBrief, error)
	Members(ctx context.Context, cubeID int64) ([]Member, error)
	Transfer(ctx context.Context, cubeID, fromUserID, toUserID int64) error
	Dissolve(ctx context.Context, cubeID, userID int64) error
	RemoveMember(ctx context.Context, cubeID, operatorID, targetID int64, reason string) error
	Leave(ctx context.Context, cubeID, userID int64) error
	// HasActiveMembership 用户是否属于任一正常立方（circle「已加入立方」前提）。
	HasActiveMembership(ctx context.Context, userID int64) (bool, error)

	// ---- 协作实体（§2）----
	CreateProject(ctx context.Context, cubeID, userID int64, in ProjectInput) (Project, error)
	ListProjects(ctx context.Context, cubeID int64, typ int16) ([]Project, error)
	CloseProject(ctx context.Context, cubeID, projectID, userID int64) error
	ReopenProject(ctx context.Context, cubeID, projectID, userID int64) error
	AddResearch(ctx context.Context, cubeID, projectID, userID int64, content, decision string) (Research, error)
	ListResearch(ctx context.Context, cubeID, projectID, userID int64) ([]Research, error)
	CreateTask(ctx context.Context, cubeID, userID int64, in TaskInput) (Task, error)
	ListTasks(ctx context.Context, cubeID, projectID int64) ([]Task, error)
	StartTask(ctx context.Context, taskID, userID int64) error
	SubmitTask(ctx context.Context, taskID, userID int64, progress int16) error
	ConfirmTask(ctx context.Context, taskID, userID int64, approve bool, reason string) error
	ReopenTask(ctx context.Context, taskID, userID int64) error
	SubmitReport(ctx context.Context, cubeID, userID int64, in ReportInput) (Report, bool, error)
	ListReports(ctx context.Context, cubeID, projectID int64) ([]Report, error)
	ConfirmReport(ctx context.Context, reportID, userID int64) error
	RejectReport(ctx context.Context, reportID, userID int64, reason string) error

	// ---- 订阅与分摊（§3）----
	Plans(ctx context.Context) ([]Plan, error)
	// ChangePlan 购买/升级订阅，返回订单号与金额（履约在支付成功后）。
	ChangePlan(ctx context.Context, cubeID, userID int64, level int16, years int) (orderID int64, amountFen int64, err error)
	InitiateApportion(ctx context.Context, cubeID, userID int64, level int16, n int, memberIDs []int64) (Apportion, error)
	PledgeApportion(ctx context.Context, cubeID, groupID, userID int64) (orderID int64, err error)
	ExitApportion(ctx context.Context, cubeID, groupID, userID int64) error
	ApportionOf(ctx context.Context, groupID int64) (Apportion, error)
	// FulfillOrder 供 trade OrderFulfiller 调用（biz_type=cube_plan）。
	FulfillOrder(ctx context.Context, bizRef string, snapshot []byte) error
}

// ---- 订阅与分摊（§3）----

// Plan 订阅档位（settings 后台可配）。
type Plan struct {
	Level       int16  `json:"level"`
	Name        string `json:"name"`
	CubeLimit   int    `json:"cube_limit"`
	MemberLimit int    `json:"member_limit"`
	PriceFen    int64  `json:"price_fen"`
}

// Apportion 分摊组。
type Apportion struct {
	ID           int64     `json:"id,string"`
	CubeID       int64     `json:"cube_id,string"`
	PerFen       int64     `json:"per_fen"`
	TargetN      int       `json:"target_n"`
	PledgedCount int       `json:"pledged_count"`
	Deadline     time.Time `json:"deadline"`
	Status       int16     `json:"status"`
}
