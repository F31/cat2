// Package contract social 领域事件（附录 C）。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// 事件 Topic。
const (
	EventFollowed          = "social.Followed.v1"
	EventUnfollowed        = "social.Unfollowed.v1"
	EventFolloweePublished = "social.FolloweePublished.v1"
)

// FollowPayload 关注/取关载荷。
type FollowPayload struct {
	FollowerID int64  `json:"follower_id,string"`
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id,string"`
}

// FolloweePublishedPayload 被关注者发布内容（供 notice 消费为系统通知）。
type FolloweePublishedPayload struct {
	UserID   int64  `json:"user_id,string"`   // 接收者（关注者）
	AuthorID int64  `json:"author_id,string"` // 发布者
	ItemID   int64  `json:"item_id,string"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
}

func newEvent(id, topic string, p any) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: topic, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}

func NewFollowed(id string, p FollowPayload) eventbus.Event { return newEvent(id, EventFollowed, p) }
func NewUnfollowed(id string, p FollowPayload) eventbus.Event {
	return newEvent(id, EventUnfollowed, p)
}
func NewFolloweePublished(id string, p FolloweePublishedPayload) eventbus.Event {
	return newEvent(id, EventFolloweePublished, p)
}
