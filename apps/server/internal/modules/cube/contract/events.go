// Package contract cube 领域事件（附录 C）。
package contract

import (
	"encoding/json"
	"time"

	"catcube/internal/kernel/eventbus"
)

// 事件 Topic。
const (
	EventCubeCreated       = "cube.CubeCreated.v1"
	EventMemberJoined      = "cube.MemberJoined.v1"
	EventMemberRemoved     = "cube.MemberRemoved.v1"
	EventCubeDissolved     = "cube.CubeDissolved.v1"
	EventProjectCreated    = "cube.ProjectCreated.v1"
	EventProjectClosed     = "cube.ProjectClosed.v1"
	EventProjectReopened   = "cube.ProjectReopened.v1"
	EventTaskAssigned      = "cube.TaskAssigned.v1"
	EventTaskSubmitted     = "cube.TaskSubmitted.v1"
	EventTaskConfirmed     = "cube.TaskConfirmed.v1"
	EventReportSubmitted   = "cube.ReportSubmitted.v1"
	EventReportConfirmed   = "cube.ReportConfirmed.v1"
	EventReportRejected    = "cube.ReportRejected.v1"
	EventApportionFormed   = "cube.ApportionFormed.v1"
	EventApportionFailed   = "cube.ApportionFailed.v1"
)

// CubePayload 立方事件载荷。
type CubePayload struct {
	CubeID   int64  `json:"cube_id,string"`
	Name     string `json:"name,omitempty"`
	UserID   int64  `json:"user_id,string,omitempty"`
	LeaderID int64  `json:"leader_id,string,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// MemberPayload 成员事件载荷。
type MemberPayload struct {
	CubeID     int64  `json:"cube_id,string"`
	UserID     int64  `json:"user_id,string"`
	Role       int16  `json:"role,omitempty"`
	OperatorID int64  `json:"operator_id,string,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// ProjectPayload 立方内项目事件载荷。
type ProjectPayload struct {
	CubeID    int64  `json:"cube_id,string"`
	ProjectID int64  `json:"project_id,string"`
	Name      string `json:"name,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
	OperatorID int64 `json:"operator_id,string,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// TaskPayload 任务事件载荷。
type TaskPayload struct {
	CubeID    int64  `json:"cube_id,string"`
	TaskID    int64  `json:"task_id,string"`
	ProjectID int64  `json:"project_id,string,omitempty"`
	UserID    int64  `json:"user_id,string"`     // 接收者（负责人/确认人）
	OperatorID int64 `json:"operator_id,string,omitempty"`
	Status    int16  `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

// ReportPayload 报备事件载荷。
type ReportPayload struct {
	CubeID    int64  `json:"cube_id,string"`
	ReportID  int64  `json:"report_id,string"`
	ProjectID int64  `json:"project_id,string,omitempty"`
	UserID    int64  `json:"user_id,string"`
	Reason    string `json:"reason,omitempty"`
}

// ApportionPayload 分摊事件载荷。
type ApportionPayload struct {
	CubeID    int64 `json:"cube_id,string"`
	GroupID   int64 `json:"group_id,string"`
	OrderID   int64 `json:"order_id,string,omitempty"`
	TargetN   int   `json:"target_n,omitempty"`
	Level     int16 `json:"level,omitempty"`
}

func newEvent(id, topic string, p any) eventbus.Event {
	payload, _ := json.Marshal(p)
	return eventbus.Event{ID: id, Topic: topic, OccurredAt: time.Now().UTC(), SpaceID: 1, Payload: payload}
}

func NewCubeCreated(id string, p CubePayload) eventbus.Event { return newEvent(id, EventCubeCreated, p) }
func NewMemberJoined(id string, p MemberPayload) eventbus.Event {
	return newEvent(id, EventMemberJoined, p)
}
func NewMemberRemoved(id string, p MemberPayload) eventbus.Event {
	return newEvent(id, EventMemberRemoved, p)
}
func NewCubeDissolved(id string, p CubePayload) eventbus.Event {
	return newEvent(id, EventCubeDissolved, p)
}
func NewProjectCreated(id string, p ProjectPayload) eventbus.Event {
	return newEvent(id, EventProjectCreated, p)
}
func NewProjectClosed(id string, p ProjectPayload) eventbus.Event {
	return newEvent(id, EventProjectClosed, p)
}
func NewProjectReopened(id string, p ProjectPayload) eventbus.Event {
	return newEvent(id, EventProjectReopened, p)
}
func NewTaskAssigned(id string, p TaskPayload) eventbus.Event { return newEvent(id, EventTaskAssigned, p) }
func NewTaskSubmitted(id string, p TaskPayload) eventbus.Event { return newEvent(id, EventTaskSubmitted, p) }
func NewTaskConfirmed(id string, p TaskPayload) eventbus.Event { return newEvent(id, EventTaskConfirmed, p) }
func NewReportSubmitted(id string, p ReportPayload) eventbus.Event {
	return newEvent(id, EventReportSubmitted, p)
}
func NewReportConfirmed(id string, p ReportPayload) eventbus.Event {
	return newEvent(id, EventReportConfirmed, p)
}
func NewReportRejected(id string, p ReportPayload) eventbus.Event {
	return newEvent(id, EventReportRejected, p)
}
func NewApportionFormed(id string, p ApportionPayload) eventbus.Event {
	return newEvent(id, EventApportionFormed, p)
}
func NewApportionFailed(id string, p ApportionPayload) eventbus.Event {
	return newEvent(id, EventApportionFailed, p)
}
