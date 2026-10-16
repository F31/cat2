// Package contract verify 对外契约：认证申请、标识查询、实名能力查询。
package contract

import (
	"context"
	"time"

	"catcube/internal/shared/entityref"
)

// 认证类型（PRD 8.4 五类）。
const (
	CertRealname         int16 = 1 // 实名（实时二要素，system 自动核验）
	CertPersonal         int16 = 2 // 个人（资质/专业认证）
	CertPersonEnterprise int16 = 3 // 个人企业身份（选已入驻企业 + 职务 + 任职证明，企业审）
	CertEnterprise       int16 = 4 // 企业（营业执照 + 法人身份证，平台审）
	CertInstitution      int16 = 5 // 机构（证件/批文 + 负责人证明 + 授权书，平台审）
)

// 核对者类型。
const (
	ReviewerPlatform int16 = 1 // 平台
	ReviewerOrg      int16 = 2 // 企业/组织
)

// 归属核验状态。
const (
	AffPending  int16 = 0
	AffVerified int16 = 1
	AffRejected int16 = 2
	AffRevoked  int16 = 3
)

// 组织管理员状态。
const (
	OrgAdminPending int16 = 0
	OrgAdminActive  int16 = 1
	OrgAdminRemoved int16 = 2
)

// 认证状态。
const (
	CertPending  int16 = 0 // 待审核/核验中
	CertApproved int16 = 1 // 已通过
	CertRejected int16 = 2 // 已驳回/核验失败
)

// 标识编码。
const (
	BadgeRealname = "realname" // 实名
	BadgeVerified = "verified" // 已认证（企业/机构）
	BadgeOfficial = "official" // 猫平方官方合作认证
)

// Cert 认证申请。
type Cert struct {
	ID           int64      `json:"id,string"`
	UserID       int64      `json:"user_id,string"`
	CertType     int16      `json:"cert_type"`
	TargetOrgID  int64      `json:"target_org_id,string,omitempty"`
	Status       int16      `json:"status"`
	RejectReason string     `json:"reject_reason,omitempty"`
	SubmittedAt  time.Time  `json:"submitted_at"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
}

// CertifyInput 认证申请入参（一期实名：姓名 + 身份证号 + 协议勾选）。
type CertifyInput struct {
	CertType        int16             `json:"cert_type"`
	RealName        string            `json:"real_name"`
	IDCardNo        string            `json:"id_card_no"`
	Agreed          bool              `json:"agreed"`
	TargetOrgID     int64             `json:"target_org_id,string"`
	MaterialFileIDs []int64           `json:"material_file_ids"`
	Extra           map[string]string `json:"extra"`
}

// Badge 标识（红框=已核验/置灰=未核验）。
type Badge struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Level int16  `json:"level"`
}

// Affiliation 个人-组织归属（一人多单位）。
type Affiliation struct {
	ID           int64  `json:"id,string"`
	UserID       int64  `json:"user_id,string"`
	OrgID        int64  `json:"org_id,string"`
	Role         string `json:"role,omitempty"`
	IsPrimary    bool   `json:"is_primary"`
	Status       int16  `json:"status"`
	RejectReason string `json:"reject_reason,omitempty"`
}

// OrgAdmin 组织管理员。
type OrgAdmin struct {
	OrgID       int64 `json:"org_id,string"`
	UserID      int64 `json:"user_id,string"`
	Status      int16 `json:"status"`
	ConfirmedAt int64 `json:"confirmed_at,omitempty"`
}
type Facade interface {
	// Certify 提交认证（一期实名：第三方二要素实时核验；通过即授予 Badge）。
	Certify(ctx context.Context, userID int64, in CertifyInput) (Cert, error)
	// CertsOf 某账号的认证记录。
	CertsOf(ctx context.Context, userID int64) ([]Cert, error)
	// BadgesOf 某实体的标识（user/org）。
	BadgesOf(ctx context.Context, ref entityref.Ref) ([]Badge, error)
	// IsRealname 是否已实名（能力查询）。
	IsRealname(ctx context.Context, userID int64) (bool, error)

	// ReviewCert 审核认证（平台或组织管理员；通过授 Badge，企业类认证通过时申请人成为组织管理员）。
	ReviewCert(ctx context.Context, certID, reviewerID int64, reviewerType int16, pass bool, reason string) error
	// PendingCerts 待审认证（平台视角 / 组织视角）。
	PendingCerts(ctx context.Context, reviewerType int16, orgID int64, limit int) ([]Cert, error)

	// ApplyAffiliation 申请加入组织（一人多单位，需组织管理员核验）。
	ApplyAffiliation(ctx context.Context, userID, orgID int64, role string, proofFileIDs []int64) (Affiliation, error)
	// AffiliationsOf 某账号的归属（含置灰/红框标识所需状态）。
	AffiliationsOf(ctx context.Context, userID int64) ([]Affiliation, error)
	// VerifyAffiliation 核验归属（组织管理员；先到先处理）。
	VerifyAffiliation(ctx context.Context, affID, reviewerID int64, pass bool, reason string) error
	// SetPrimaryAffiliation 设为主单位（唯一）。
	SetPrimaryAffiliation(ctx context.Context, userID, affID int64) error
	// PendingAffiliations 某组织待核验归属（组织管理员视角；红点数据）。
	PendingAffiliations(ctx context.Context, orgID, reviewerID int64, limit int) ([]Affiliation, error)
	// ResolveAffiliation 平台代处理（组织管理员超 7 天未处理）。
	ResolveAffiliation(ctx context.Context, affID, operatorID int64, pass bool, reason string) error

	// InviteOrgAdmin 邀请组织管理员（需对方确认）。
	InviteOrgAdmin(ctx context.Context, orgID, inviterID, inviteeID int64) error
	// ConfirmOrgAdmin 被邀请人确认成为管理员。
	ConfirmOrgAdmin(ctx context.Context, inviteID, userID int64) error
	// OrgAdminsOf 组织管理员列表。
	OrgAdminsOf(ctx context.Context, orgID int64) ([]OrgAdmin, error)
	// ResetOrgAdmin 平台重置组织管理员（失联代处理）。
	ResetOrgAdmin(ctx context.Context, orgID, operatorID, userID int64, remove bool) error
	// IsOrgAdmin 是否在任组织管理员（核验权限判定）。
	IsOrgAdmin(ctx context.Context, orgID, userID int64) (bool, error)
}
