// Package contract identity 对外契约：认证、会话、用户摘要、注销。
package contract

import "context"

// UserBrief 用户摘要（供其他模块展示）。
type UserBrief struct {
	ID       int64  `json:"id,string"`
	Nickname string `json:"nickname"`
	AvatarID int64  `json:"avatar_file_id,string,omitempty"`
	Status   int16  `json:"status"` // 0正常/1限制/2封禁/3已注销
}

// Tokens 令牌对。
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // Access 有效期秒（2h）
}

// Facade identity 对外接口。
type Facade interface {
	// Authenticate 校验 Access Token，返回用户 ID 与授权版本。
	Authenticate(ctx context.Context, accessToken string) (userID int64, authzVer int, err error)
	SessionOf(ctx context.Context, userID int64) (activeSessions int, err error)
	UserBrief(ctx context.Context, userID int64) (UserBrief, error)
	// Deactivate 注销并匿名化（发布 identity.UserDeactivated，各模块自行处理本模块数据）。
	Deactivate(ctx context.Context, userID int64) error
	// AuthzBump 递增授权版本（会员到期/移出立方/封禁等触发，消除陈旧窗口）。
	AuthzBump(ctx context.Context, userID int64) error
}
