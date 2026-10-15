// Package contract msg 领域事件。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// EventMessageSent 消息发送事件 Topic（供 notice 触达；不含手机号/证件号等敏感字段）。
const EventMessageSent = "msg.MessageSent.v1"

// MessageSentPayload 消息发送载荷。
type MessageSentPayload struct {
	MessageID   int64  `json:"message_id,string"`
	SessionID   int64  `json:"session_id,string"`
	SessionKind int16  `json:"session_kind"`
	SenderID    int64  `json:"sender_id,string"`
	MsgType     int16  `json:"msg_type"`
	Seq         int64  `json:"seq"`
	Content     string `json:"content"`
}

// NewMessageSent 构造 MessageSent 事件。
func NewMessageSent(id string, p MessageSentPayload) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: EventMessageSent, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}
