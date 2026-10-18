// Package contract circle 领域事件（附录 C）。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// 事件 Topic。
const (
	EventCircleCreated   = "circle.CircleCreated.v1"
	EventCircleEnded     = "circle.CircleEnded.v1"
	EventCircleDissolved = "circle.CircleDissolved.v1"
	EventMemberJoined    = "circle.MemberJoined.v1"
	EventJoinApplied     = "circle.JoinApplied.v1"
	EventJoinApproved    = "circle.JoinApproved.v1"
	EventJoinRejected    = "circle.JoinRejected.v1"
	EventDemandPublished = "circle.DemandPublished.v1"
	EventDemandExpired   = "circle.DemandExpired.v1"
)

// CirclePayload 圈子事件载荷。
type CirclePayload struct {
	CircleID int64  `json:"circle_id,string"`
	Name     string `json:"name,omitempty"`
	OwnerID  int64  `json:"owner_id,string,omitempty"`
	UserID   int64  `json:"user_id,string,omitempty"`
}

// MemberPayload 成员事件载荷。
type MemberPayload struct {
	CircleID int64 `json:"circle_id,string"`
	UserID   int64 `json:"user_id,string"`
	Via      int16 `json:"via,omitempty"`
}

// ApplyPayload 入圈申请事件载荷。
type ApplyPayload struct {
	CircleID int64  `json:"circle_id,string"`
	Name     string `json:"name,omitempty"`
	UserID   int64  `json:"user_id,string"`
	OwnerID  int64  `json:"owner_id,string,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// DemandPayload 需求事件载荷。
type DemandPayload struct {
	CircleID int64  `json:"circle_id,string"`
	DemandID int64  `json:"demand_id,string"`
	AuthorID int64  `json:"author_id,string"`
	Title    string `json:"title,omitempty"`
	Scope    int16  `json:"scope,omitempty"`
}

func newEvent(id, topic string, p any) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: topic, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}

func NewCircleCreated(id string, p CirclePayload) eventbus.Event {
	return newEvent(id, EventCircleCreated, p)
}
func NewCircleEnded(id string, p CirclePayload) eventbus.Event {
	return newEvent(id, EventCircleEnded, p)
}
func NewCircleDissolved(id string, p CirclePayload) eventbus.Event {
	return newEvent(id, EventCircleDissolved, p)
}
func NewMemberJoined(id string, p MemberPayload) eventbus.Event {
	return newEvent(id, EventMemberJoined, p)
}
func NewJoinApplied(id string, p ApplyPayload) eventbus.Event {
	return newEvent(id, EventJoinApplied, p)
}
func NewJoinApproved(id string, p ApplyPayload) eventbus.Event {
	return newEvent(id, EventJoinApproved, p)
}
func NewJoinRejected(id string, p ApplyPayload) eventbus.Event {
	return newEvent(id, EventJoinRejected, p)
}
func NewDemandPublished(id string, p DemandPayload) eventbus.Event {
	return newEvent(id, EventDemandPublished, p)
}
func NewDemandExpired(id string, p DemandPayload) eventbus.Event {
	return newEvent(id, EventDemandExpired, p)
}
