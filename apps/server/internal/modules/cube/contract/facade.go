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
}
