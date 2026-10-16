// Package contract circle 对外契约：圈子摘要/角色与圈内发布。
package contract

import (
	"context"
	"time"
)

// 圈子状态（状态机 #10）。
const (
	CircleActive    int16 = 0
	CircleEnded     int16 = 1
	CircleDissolved int16 = 2
)

// 圈类型。
const (
	TypePermanent int16 = 1 // 常设
	TypeTemporary int16 = 2 // 临时
)

// 成员加入途径。
const (
	ViaApply  int16 = 1
	ViaInvite int16 = 2
	ViaOrg    int16 = 3
	ViaEvent  int16 = 4
)

// 圈内需求状态（状态机 #11）。
const (
	DemandValid   int16 = 0
	DemandExpired int16 = 1
	DemandDeleted int16 = 2
)

// CircleBrief 圈子摘要。
type CircleBrief struct {
	ID          int64      `json:"id,string"`
	Name        string     `json:"name"`
	Type        int16      `json:"type"`
	Status      int16      `json:"status"`
	OwnerUserID int64      `json:"owner_user_id,string"`
	MemberCount int        `json:"member_count"`
	EndAt       *time.Time `json:"end_at,omitempty"`
}

// CircleDetail 圈子详情。
type CircleDetail struct {
	CircleBrief
	Intro         string   `json:"intro,omitempty"`
	CoverFileID   int64    `json:"cover_file_id,string,omitempty"`
	RegionCode    string   `json:"region_code,omitempty"`
	IndustryCodes []string `json:"industry_codes"`
	MyRole        int16    `json:"my_role"` // 0非成员/1圈主/2成员
	Joined        bool     `json:"joined"`
	Recommendable bool     `json:"recommendable"` // 新圈 7 天内不进推荐流
}

// Demand 圈内需求。
type Demand struct {
	ID            int64     `json:"id,string"`
	CircleID      int64     `json:"circle_id,string"`
	AuthorID      int64     `json:"author_id,string"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	DemandType    string    `json:"demand_type,omitempty"`
	RegionCode    string    `json:"region_code,omitempty"`
	IndustryCodes []string  `json:"industry_codes"`
	ExpireAt      time.Time `json:"expire_at"`
	Scope         int16     `json:"scope"` // 1仅本圈/2全平台
	Status        int16     `json:"status"`
	ContactPhone  string    `json:"contact_phone,omitempty"` // 会员可见
	TransferRef   string    `json:"transfer_ref,omitempty"`  // 已转入猫立方办的项目引用
	CreatedAt     time.Time `json:"created_at"`
}

// CreateInput 建圈入参。
type CreateInput struct {
	Name          string
	Intro         string
	CoverFileID   int64
	RegionCode    string
	IndustryCodes []string
	Type          int16
	DurationDays  int
	OrgID         int64
}

// DemandInput 发布需求入参。
type DemandInput struct {
	Title         string
	Content       string
	DemandType    string
	RegionCode    string
	IndustryCodes []string
	Days          int
	Scope         int16
	ContactPhone  string
}

// Facade circle 对外接口。
type Facade interface {
	Brief(ctx context.Context, circleID int64) (CircleBrief, error)
	RoleOf(ctx context.Context, circleID, userID int64) (int16, error)
	IsMember(ctx context.Context, circleID, userID int64) (bool, error)
	ListMine(ctx context.Context, userID int64) ([]CircleBrief, error)
	Detail(ctx context.Context, viewerID, circleID int64) (CircleDetail, error)
	Create(ctx context.Context, userID int64, in CreateInput) (CircleBrief, error)
	Apply(ctx context.Context, circleID, userID int64) error
	ApproveApply(ctx context.Context, circleID, ownerID, applicantID int64) error
	InviteCode(ctx context.Context, circleID, userID int64) (string, time.Time, error)
	JoinByInvite(ctx context.Context, userID int64, code string) (CircleBrief, error)
	Members(ctx context.Context, circleID int64) ([]int64, error)
	Dissolve(ctx context.Context, circleID, userID int64) error
	ConvertToPermanent(ctx context.Context, circleID, userID int64) error
	EndDue(ctx context.Context) error

	// 圈内发布与挂载（§6）
	PublishDemand(ctx context.Context, circleID, userID int64, in DemandInput) (Demand, error)
	ListDemands(ctx context.Context, circleID, viewerID int64, limit int) ([]Demand, error)
	RenewDemand(ctx context.Context, demandID, userID int64) error
	PublishFeed(ctx context.Context, circleID, userID int64, text string, imageFileIDs []int64, announcement bool) (int64, error)
	ExpireDemands(ctx context.Context) error

	// 挂载与到猫立方里办（§6.6/6.7）
	Mount(ctx context.Context, circleID, userID int64, itemRef string) (Mount, bool, error)
	ListMounts(ctx context.Context, circleID int64) ([]Mount, error)
	DecideMount(ctx context.Context, mountID, userID int64, approve bool) error
	ToCube(ctx context.Context, circleID, demandID, cubeID, userID int64) (int64, error)
}

// Mount 挂载记录。
type Mount struct {
	ID             int64     `json:"id,string"`
	CircleID       int64     `json:"circle_id,string"`
	ItemRef        string    `json:"item_ref"`
	MountedBy      int64     `json:"mounted_by,string"`
	MountType      int16     `json:"mount_type"`      // 1本人/2申请挂载
	ApprovalStatus int16     `json:"approval_status"` // 0待确认/1已同意/2已拒绝
	Status         int16     `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}
