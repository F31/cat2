// Package contract profile 领域事件（附录 C）。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// 事件 Topic（命名 module.PastTense.vN，载荷不含敏感字段）。
const (
	EventProfileUpdated = "profile.ProfileUpdated.v1"
	EventOrgCardUpdated = "profile.OrgCardUpdated.v1"
)

// MatchProfile 匹配画像（事件载荷子集：区域/行业/需求/供给），discovery 据此建倒排。
type MatchProfile struct {
	Region   string   `json:"region,omitempty"`
	Industry []string `json:"industry,omitempty"`
	Need     []string `json:"need,omitempty"`
	Offer    []string `json:"offer,omitempty"`
}

// ProfileUpdatedPayload ProfileUpdated / OrgCardUpdated 载荷。
// search_doc_version 随事件携带，供 discovery 投影防旧事件覆盖。
type ProfileUpdatedPayload struct {
	UserID           int64        `json:"user_id,string"`
	SubjectTypeCode  string       `json:"subject_type_code,omitempty"`
	Name             string       `json:"name,omitempty"`
	ProfileTier      int16        `json:"profile_tier"`
	SearchDocVersion int          `json:"search_doc_version"`
	Match            MatchProfile `json:"match_profile"`
}

// NewProfileUpdated 构造 ProfileUpdated 事件（个人名片）。
func NewProfileUpdated(id string, p ProfileUpdatedPayload) eventbus.Event {
	return newEvent(id, EventProfileUpdated, p)
}

// NewOrgCardUpdated 构造 OrgCardUpdated 事件（组织名片）。
func NewOrgCardUpdated(id string, p ProfileUpdatedPayload) eventbus.Event {
	return newEvent(id, EventOrgCardUpdated, p)
}

func newEvent(id, topic string, p ProfileUpdatedPayload) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{
		ID:         id,
		Topic:      topic,
		OccurredAt: time.Now().UTC(),
		SpaceID:    1,
		Payload:    payload,
	}
}
