// Package contract trust 领域事件（附录 C）。载荷不含敏感字段。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// 事件 Topic。
const (
	EventModerationPassed   = "trust.ModerationPassed.v1"
	EventModerationRejected = "trust.ModerationRejected.v1"
	EventReportConfirmed    = "trust.ReportConfirmed.v1"
	EventViolationRecorded  = "trust.ViolationRecorded.v1"
	EventAccountRestricted  = "trust.AccountRestricted.v1"
)

// ModerationPayload 审核结论载荷。
type ModerationPayload struct {
	SubjectType string `json:"subject_type"`
	SubjectID   int64  `json:"subject_id,string,omitempty"`
	ReasonCode  string `json:"reason_code,omitempty"`
}

// ReportConfirmedPayload 举报成立载荷。
type ReportConfirmedPayload struct {
	ReportID   int64  `json:"report_id,string"`
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id,string"`
	ReasonCode string `json:"reason_code"`
}

// ViolationPayload 违规记分载荷。
type ViolationPayload struct {
	UserID       int64 `json:"user_id,string"`
	Points       int   `json:"points"`
	Total        int   `json:"total"`
	SourceTaskID int64 `json:"source_task_id,string,omitempty"`
}

// AccountRestrictedPayload 账号限制/封禁载荷。
type AccountRestrictedPayload struct {
	UserID int64  `json:"user_id,string"`
	Type   int16  `json:"type"` // 1限制发布/2封禁
	Days   int    `json:"days"`
	Reason string `json:"reason,omitempty"`
}

func newEvent(id, topic string, p any) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: topic, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}

func NewModerationPassed(id string, p ModerationPayload) eventbus.Event {
	return newEvent(id, EventModerationPassed, p)
}
func NewModerationRejected(id string, p ModerationPayload) eventbus.Event {
	return newEvent(id, EventModerationRejected, p)
}
func NewReportConfirmed(id string, p ReportConfirmedPayload) eventbus.Event {
	return newEvent(id, EventReportConfirmed, p)
}
func NewViolationRecorded(id string, p ViolationPayload) eventbus.Event {
	return newEvent(id, EventViolationRecorded, p)
}
func NewAccountRestricted(id string, p AccountRestrictedPayload) eventbus.Event {
	return newEvent(id, EventAccountRestricted, p)
}
