// Package contract membership 领域事件（附录 C）。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// 事件 Topic。
const (
	EventMemberActivated   = "membership.MemberActivated.v1"
	EventMemberExpired     = "membership.MemberExpired.v1"
	EventSubAccountChanged = "membership.SubAccountChanged.v1"
)

// MemberPayload 会员事件载荷。
type MemberPayload struct {
	UserID    int64  `json:"user_id,string"`
	Level     int16  `json:"level"`
	ExpiresAt string `json:"expires_at"`
}

// SubAccountPayload 子账号变更载荷。
type SubAccountPayload struct {
	MasterID  int64  `json:"master_id,string"`
	SubUserID int64  `json:"sub_user_id,string"`
	Action    string `json:"action"` // invite/remove
}

func newEvent(id, topic string, p any) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: topic, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}

func NewMemberActivated(id string, p MemberPayload) eventbus.Event {
	return newEvent(id, EventMemberActivated, p)
}
func NewMemberExpired(id string, p MemberPayload) eventbus.Event {
	return newEvent(id, EventMemberExpired, p)
}
func NewSubAccountChanged(id string, p SubAccountPayload) eventbus.Event {
	return newEvent(id, EventSubAccountChanged, p)
}
