// Package contract identity 对外契约：认证、会话、用户摘要、注销。
package contract

import "context"

// UserBrief 用户摘要（供其他模块展示）。
type UserBrief struct {
	ID           int64  `json:"id,string"`
	Nickname     string `json:"nickname"`
	AvatarID     int64  `json:"avatar_file_id,string,omitempty"`
	Status       int16  `json:"status"`        // 0正常/1限制/2封禁/3已注销
	IdentityType int16  `json:"identity_type"` // 1个人/2组织
	AuthzVer     int    `json:"authz_ver"`     // 授权版本（供外部中间件比对陈旧窗口）
}

// Tokens 令牌对。
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // Access 有效期秒（2h）
}

// AdminUserRow 后台用户检索行（仅掩码/盲索引，无明文）。
type AdminUserRow struct {
	ID             int64  `json:"id,string"`
	Nickname       string `json:"nickname"`
	Status         int16  `json:"status"`
	IdentityType   int16  `json:"identity_type"`
	RealnameStatus int16  `json:"realname_status"`
	PhoneMasked    string `json:"phone_masked"`
	RealName       string `json:"real_name,omitempty"`
	IDCardMasked   string `json:"id_card_masked,omitempty"`
	RegionCode     string `json:"region_code,omitempty"`
	CreatedAt      int64  `json:"created_at"`
}

// AdminUserFilter 后台检索条件（手机号走盲索引精确匹配；-1/0 表示不限）。
type AdminUserFilter struct {
	Status         int16  `json:"status"`
	Q              string `json:"q"`
	Phone          string `json:"phone"`
	RealnameStatus int16  `json:"realname_status"`
	IdentityType   int16  `json:"identity_type"`
	FromUnix       int64  `json:"from"`
	ToUnix         int64  `json:"to"`
	Limit          int    `json:"limit"`
	Offset         int    `json:"offset"`
}

// Facade identity 对外接口。
type Facade interface {
	// IsAdmin 是否平台管理员（后台路由硬门槛；p4 细化五角色）。
	IsAdmin(ctx context.Context, userID int64) (bool, error)
	// AdminStepUpRequired 后台账号是否需短信二次验证（登录失败达阈值；p4 §3.4）。
	AdminStepUpRequired(ctx context.Context, userID int64) (bool, error)
	// SendStepUpCode 发送后台二次验证短信（复用登录短信通道）。
	SendStepUpCode(ctx context.Context, userID int64) error
	// VerifyStepUpCode 校验后台二次验证码；成功清除失败计数。
	VerifyStepUpCode(ctx context.Context, userID int64, code string) error
	// SendPasswordResetCode 发送后台找回密码验证码。
	SendPasswordResetCode(ctx context.Context, userID int64) error
	// VerifyPasswordResetCode 校验后台找回密码验证码（一次性）。
	VerifyPasswordResetCode(ctx context.Context, userID int64, code string) error
	// GrantAdmin 授予/取消平台管理员（运维引导）。
	GrantAdmin(ctx context.Context, userID int64, admin bool) error
	// Authenticate 校验 Access Token，返回用户 ID 与授权版本。
	Authenticate(ctx context.Context, accessToken string) (userID int64, authzVer int, err error)
	SessionOf(ctx context.Context, userID int64) (activeSessions int, err error)
	UserBrief(ctx context.Context, userID int64) (UserBrief, error)
	// OpenID 返回用户当前微信 openid（支付 JSAPI 使用；空表示未绑定）。
	OpenID(ctx context.Context, userID int64) (string, error)
	// Deactivate 注销并匿名化（发布 identity.UserDeactivated，各模块自行处理本模块数据）。
	Deactivate(ctx context.Context, userID int64) error
	// AuthzBump 递增授权版本（会员到期/移出立方/封禁等触发，消除陈旧窗口）。
	AuthzBump(ctx context.Context, userID int64) error
	// MarkRealname 回写实名核验结果（verify 经此，不读 idn_user）。status 0未实名/1已实名/2核验失败。
	MarkRealname(ctx context.Context, userID int64, status int16) error
	// AdminSearchUsers 后台用户检索（昵称模糊/手机号盲精确/状态/实名/注册时间）。
	AdminSearchUsers(ctx context.Context, f AdminUserFilter) ([]AdminUserRow, error)
	// UserIDByPhone 按手机号盲索引查已有用户（后台口令登录；不建号）。
	UserIDByPhone(ctx context.Context, phone string) (int64, error)
	// IssueTokens 为已认证用户签发会话（后台口令登录）。
	IssueTokens(ctx context.Context, userID int64, device string) (Tokens, error)
}
