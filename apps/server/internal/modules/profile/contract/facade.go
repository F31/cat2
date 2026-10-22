// Package contract profile 对外契约：名片、画像完成度、组织摘要。
package contract

import "context"

// Card 名片对外只读 DTO（不含手机号/证件号等敏感字段；联系方式仅回传是否配置）。
type Card struct {
	UserID          int64    `json:"user_id,string"`
	SubjectTypeCode string   `json:"subject_type_code,omitempty"`
	OrgTypeCode     string   `json:"org_type_code,omitempty"`
	IdentityType    int16    `json:"identity_type"` // 1个人/2组织
	Name            string   `json:"name"`
	AvatarFileID    int64    `json:"avatar_file_id,string,omitempty"`
	RegionCode      string   `json:"region_code,omitempty"`
	IndustryCodes   []string `json:"industry_codes,omitempty"`
	NeedCodes       []string `json:"need_codes,omitempty"`
	OfferCodes      []string `json:"offer_codes,omitempty"`
	AdvantageField  string   `json:"advantage_field,omitempty"`
	Background      string   `json:"background,omitempty"`
	TargetCustomers string   `json:"target_customers,omitempty"`
	TargetChannels  string   `json:"target_channels,omitempty"`
	CoreStrength    string   `json:"core_strength,omitempty"`
	Intro           string   `json:"intro,omitempty"`
	ProfileTier     int16    `json:"profile_tier"`
	HasContact      bool     `json:"has_contact"` // 是否已配置联系方式（明文永不下发）
}

// AdminOrgInput 后台维护组织名片字段（不触碰联系方式/资质材料，避免误清空）。
type AdminOrgInput struct {
	OrgTypeCode   string   `json:"org_type_code"`
	Name          string   `json:"name"`
	RegionCode    string   `json:"region_code"`
	IndustryCodes []string `json:"industry_codes"`
	NeedCodes     []string `json:"need_codes"`
	OfferCodes    []string `json:"offer_codes"`
	CoreStrength  string   `json:"core_strength"`
	Intro         string   `json:"intro"`
}

// Completion 画像完成度（第二层五项任一为空即未完成）。
type Completion struct {
	Complete bool     `json:"complete"`
	Missing  []string `json:"missing"`
	Tier     int16    `json:"tier"` // 1第一层 / 2第二层完整 / 3含增值画像
}

// OrgBrief 组织/个人摘要（供引用解析与展示）。
type OrgBrief struct {
	UserID   int64  `json:"user_id,string"`
	Name     string `json:"name"`
	AvatarID int64  `json:"avatar_file_id,string,omitempty"`
	Region   string `json:"region_code,omitempty"`
}

// Facade profile 对外接口。
type Facade interface {
	// CardOf 取名片（对外只读视图，敏感字段已过滤）。
	CardOf(ctx context.Context, userID int64) (Card, error)
	// AdminCardOf 后台取完整名片视图（不做展示过滤；仍不含联系方式/证件号明文）。
	AdminCardOf(ctx context.Context, userID int64) (Card, error)
	// AdminUpdateOrgCard 后台维护组织名片字段（跳过认证锁定，保留联系方式和资质材料）。
	AdminUpdateOrgCard(ctx context.Context, userID int64, in AdminOrgInput) (Card, error)
	// CompletionOf 取画像完成度。
	CompletionOf(ctx context.Context, userID int64) (Completion, error)
	// OrgBrief 取摘要（引用解析/展示用）。
	OrgBrief(ctx context.Context, userID int64) (OrgBrief, error)
	// ContactPhone 取联系方式明文（未配置返回空串）；会员门槛由调用方（bff）判定，本方法不自带鉴权。
	ContactPhone(ctx context.Context, userID int64) (string, error)
}
