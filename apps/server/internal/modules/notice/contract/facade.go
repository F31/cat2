// Package contract notice 对外契约：站内信、通知偏好、订阅授权额度。
package contract

import (
	"context"
	"time"
)

// Notice 站内信。
type Notice struct {
	ID           int64      `json:"id,string"`
	TemplateCode string     `json:"template_code"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	Ref          string     `json:"ref,omitempty"`
	Channel      string     `json:"channel"`
	IsRead       bool       `json:"is_read"`
	ReadAt       *time.Time `json:"read_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Preference 通知偏好。
type Preference struct {
	TemplateCode string `json:"template_code"`
	Enabled      bool   `json:"enabled"`
	Closeable    bool   `json:"closeable"` // 资金/审核类不可关闭
}

// Facade notice 对外接口。
type Facade interface {
	List(ctx context.Context, userID int64, unreadOnly bool, limit int) ([]Notice, error)
	UnreadCount(ctx context.Context, userID int64) (int, error)
	MarkRead(ctx context.Context, userID, noticeID int64) error
	Preferences(ctx context.Context, userID int64) ([]Preference, error)
	SetPreference(ctx context.Context, userID int64, templateCode string, enabled bool) error
	ApplyQuota(ctx context.Context, userID int64, templateCode string, n int) error
}
