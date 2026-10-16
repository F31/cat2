// Package contract verify 领域事件（附录 C）。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// 事件 Topic（载荷不含姓名/证件号等敏感字段）。
const (
	EventCertificationApproved = "verify.CertificationApproved.v1"
	EventCertificationRejected = "verify.CertificationRejected.v1"
	EventAffiliationVerified   = "verify.AffiliationVerified.v1"
	EventOrgAdminChanged       = "verify.OrgAdminChanged.v1"
)

// AffiliationPayload 归属核验结果。
type AffiliationPayload struct {
	AffiliationID int64 `json:"affiliation_id,string"`
	UserID        int64 `json:"user_id,string"`
	OrgID         int64 `json:"org_id,string"`
	Status        int16 `json:"status"`
}

// OrgAdminPayload 组织管理员变更。
type OrgAdminPayload struct {
	OrgID  int64  `json:"org_id,string"`
	UserID int64  `json:"user_id,string"`
	Action string `json:"action"` // affiliation_applied / invited / confirmed / removed
}

// NewAffiliationVerified 归属核验事件。
func NewAffiliationVerified(id string, p AffiliationPayload) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: EventAffiliationVerified, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}

// NewOrgAdminChanged 组织管理员变更事件。
func NewOrgAdminChanged(id string, p OrgAdminPayload) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: EventOrgAdminChanged, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}

// CertificationPayload 认证结论载荷。
type CertificationPayload struct {
	UserID    int64  `json:"user_id,string"`
	CertType  int16  `json:"cert_type"`
	OrgID     int64  `json:"org_id,string,omitempty"`
	BadgeCode string `json:"badge_code,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// NewCertificationApproved 构造通过事件。
func NewCertificationApproved(id string, p CertificationPayload) eventbus.Event {
	return newEvent(id, EventCertificationApproved, p)
}

// NewCertificationRejected 构造驳回事件。
func NewCertificationRejected(id string, p CertificationPayload) eventbus.Event {
	return newEvent(id, EventCertificationRejected, p)
}

func newEvent(id, topic string, p CertificationPayload) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: topic, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}
