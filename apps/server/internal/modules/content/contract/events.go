// Package contract content 领域事件（附录 C）。载荷含 SearchDoc，不含敏感字段。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
	dictcontract "catcube/internal/shared/dict/contract"
)

// 事件 Topic。
const (
	EventContentSubmitted = "content.ContentSubmitted.v1"
	EventContentPublished = "content.ContentPublished.v1"
	EventContentTakenDown = "content.ContentTakenDown.v1"
	EventContentClosed    = "content.ContentClosed.v1"
	EventSignupCreated    = "content.SignupCreated.v1"
)

// SignupPayload 活动/会议报名事件（供圈子「报名即入圈」等订阅方使用）。
type SignupPayload struct {
	ItemID   int64    `json:"item_id,string"`
	Kind     string   `json:"kind"`
	OwnerRef OwnerRef `json:"owner_ref"`
	Title    string   `json:"title"`
	UserID   int64    `json:"user_id,string"`
}

// ContentPayload 内容事件载荷（含 SearchDoc 投影要素）。
type ContentPayload struct {
	ItemID           int64               `json:"item_id,string"`
	Kind             string              `json:"kind"`
	OwnerRef         OwnerRef            `json:"owner_ref"`
	Title            string              `json:"title"`
	CoverFileID      int64               `json:"cover_file_id,string,omitempty"`
	RegionCode       string              `json:"region_code,omitempty"`
	TagSet           dictcontract.TagSet `json:"tagset"`
	Status           int16               `json:"status"`
	SearchDocVersion int                 `json:"search_doc_version"`
	Reason           string              `json:"reason,omitempty"`
}

// NewEvent 构造内容事件。
func NewSignupEvent(id string, p SignupPayload) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: EventSignupCreated, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}

func NewEvent(id, topic string, p ContentPayload) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: topic, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}
