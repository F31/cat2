// Package contract identity 领域事件（附录 C）。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// 事件 Topic（命名 module.PastTense.vN，载荷不含敏感字段）。
const (
	EventUserRegistered   = "identity.UserRegistered.v1"
	EventUserDeactivated  = "identity.UserDeactivated.v1"
	EventAuthzChanged     = "identity.AuthzChanged.v1"
)

// NewEvent 构造事件载荷。
func NewEvent(id, topic string, userID int64) eventbus.Event {
	payload, _ := json.Marshal(map[string]any{"user_id": userID})
	return eventbus.Event{
		ID:         id,
		Topic:      topic,
		OccurredAt: time.Now().UTC(),
		SpaceID:    1,
		Payload:    payload,
	}
}
