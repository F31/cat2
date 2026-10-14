// Package contract verify 对外契约：认证申请、标识查询、实名能力查询。
package contract

import (
	"context"
	"time"

	"catcube/internal/shared/entityref"
)

// 认证类型（一期 verify 仅实名；2~5 预留：个人/个人企业身份/企业/机构）。
const (
	CertRealname int16 = 1
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

// Facade verify 对外接口。
type Facade interface {
	// Certify 提交认证（一期实名：第三方二要素实时核验；通过即授予 Badge）。
	Certify(ctx context.Context, userID int64, in CertifyInput) (Cert, error)
	// CertsOf 某账号的认证记录。
	CertsOf(ctx context.Context, userID int64) ([]Cert, error)
	// BadgesOf 某实体的标识（user/org）。
	BadgesOf(ctx context.Context, ref entityref.Ref) ([]Badge, error)
	// IsRealname 是否已实名（能力查询）。
	IsRealname(ctx context.Context, userID int64) (bool, error)
}
