// Package contract social 对外契约：关注/取关、收藏、未读。
package contract

import (
	"context"
	"time"
)

// Target 目标引用（EntityRef 的展示投影）。
type Target struct {
	Type string `json:"type"`
	ID   int64  `json:"id,string"`
}

// FollowItem 关注关系（含回填展示信息）。
type FollowItem struct {
	FollowerID int64     `json:"follower_id,string"`
	Target     Target    `json:"target"`
	Title      string    `json:"title,omitempty"`
	Cover      string    `json:"cover,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// FavoriteItem 收藏项（含回填展示信息）。
type FavoriteItem struct {
	ID        int64     `json:"id,string"`
	Target    Target    `json:"target"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title,omitempty"`
	Cover     string    `json:"cover,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Facade social 对外接口。
type Facade interface {
	Follow(ctx context.Context, followerID int64, target string) error
	Unfollow(ctx context.Context, followerID int64, target string) error
	IsFollowing(ctx context.Context, followerID int64, target string) (bool, error)
	// Following 我关注的人/内容。
	Following(ctx context.Context, userID int64) ([]FollowItem, error)
	// Followers 关注我的人（followee_ref=user:<id>）。
	Followers(ctx context.Context, userID int64) ([]FollowItem, error)
	Favorite(ctx context.Context, userID int64, target string) error
	Unfavorite(ctx context.Context, userID int64, target string) error
	Favorites(ctx context.Context, userID int64, kind string) ([]FavoriteItem, error)
	Unread(ctx context.Context, userID int64) (int, error)
}
