// collab.go cube 协作实体：项目（研究/承接）、任务四态、重点客户报备。
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/statemachine"
	"catcube/internal/modules/cube/contract"
	"catcube/internal/modules/cube/internal/domain"
	"catcube/internal/modules/cube/internal/infra"
	"catcube/internal/shared/entityref"
	"catcube/pkg/ids"
)

// 状态映射：DB 数值 ↔ 领域字符串。
func taskState(code int16) string {
	switch code {
	case 0:
		return domain.TaskTodo
	case 1:
		return domain.TaskDoing
	case 2:
		return domain.TaskReviewing
	default:
		return domain.TaskDone
	}
}

func taskCode(state string) int16 {
	switch state {
	case domain.TaskTodo:
		return 0
	case domain.TaskDoing:
		return 1
	case domain.TaskReviewing:
		return 2
	default:
		return 3
	}
}

func (s *Service) canManage(ctx context.Context, cubeID, userID int64) (bool, error) {
	role, err := s.RoleOf(ctx, cubeID, userID)
	if err != nil {
		return false, err
	}
	return role == contract.RoleLeader || role == contract.RoleHoney, nil
}

// ---- 项目 ----

// CreateProject 新建立方内项目（研究 type=1 / 承接 type=2）。
func (s *Service) CreateProject(ctx context.Context, cubeID, userID int64, in contract.ProjectInput) (contract.Project, error) {
	ok, err := s.canManage(ctx, cubeID, userID)
	if err != nil {
		return contract.Project{}, err
	}
	if !ok {
		return contract.Project{}, domain.ErrNoPermission
	}
	if in.Name == "" {
		return contract.Project{}, domain.ErrInvalidInput
	}
	if in.Type != 1 && in.Type != 2 {
		in.Type = 1
	}
	p := infra.Project{
		ID: int64(ids.New()), CubeID: cubeID, Type: in.Type, Name: in.Name,
		RelatedRef: in.RelatedRef, RelatedCustomer: in.RelatedCustomer, LeaderID: userID,
		Participants: in.Participants, SourceRef: in.SourceRef,
	}
	if in.ContentProjectID > 0 {
		v := in.ContentProjectID
		p.ContentProject = &v
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.CreateProject(ctx, tx, p); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewProjectCreated(ids.New().String(), contract.ProjectPayload{
			CubeID: cubeID, ProjectID: p.ID, Name: p.Name, SourceRef: p.SourceRef, OperatorID: userID,
		}))
	})
	if err != nil {
		slog.Error("cube 建项目失败", "err", err)
		return contract.Project{}, err
	}
	if s.msg != nil {
		_, _ = s.msg.EnsureGroup(ctx, "cube.project:"+strconv.FormatInt(p.ID, 10), p.Name, userID)
	}
	return toProject(p), nil
}

// ListProjects 项目列表（typ<=0 全部）。
func (s *Service) ListProjects(ctx context.Context, cubeID int64, typ int16) ([]contract.Project, error) {
	rows, err := s.repo.ListProjects(ctx, cubeID, typ, -1)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Project, 0, len(rows))
	for _, p := range rows {
		cp := toProject(p)
		cp.PlatformStatus = s.platformStatus(ctx, cp.ContentProjectID)
		out = append(out, cp)
	}
	return out, nil
}

// CloseProject 关闭项目。
func (s *Service) CloseProject(ctx context.Context, cubeID, projectID, userID int64) error {
	return s.projectTransition(ctx, cubeID, projectID, userID, domain.EventClose, 1)
}

// ReopenProject 重开项目。
func (s *Service) ReopenProject(ctx context.Context, cubeID, projectID, userID int64) error {
	return s.projectTransition(ctx, cubeID, projectID, userID, domain.EventReopen, 0)
}

func (s *Service) projectTransition(ctx context.Context, cubeID, projectID, userID int64, event string, target int16) error {
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if p.CubeID != cubeID {
		return domain.ErrNotFound
	}
	ok, err := s.canManage(ctx, cubeID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNoPermission
	}
	from := domain.ProjectActive
	if p.Status == 1 {
		from = domain.ProjectClosed
	}
	if _, err := statemachine.NewEngine(nil).Apply(ctx, domain.MachineProject, projectID, from, event, "leader"); err != nil {
		return domain.ErrTaskTransition
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.SetProjectStatus(ctx, tx, projectID, target); err != nil {
			return err
		}
		ev := contract.NewProjectClosed
		if event == domain.EventReopen {
			ev = contract.NewProjectReopened
		}
		return s.publish(ctx, tx, ev(ids.New().String(), contract.ProjectPayload{
			CubeID: cubeID, ProjectID: projectID, Name: p.Name, OperatorID: userID,
		}))
	})
	return err
}

// AddResearch 新增研究记录（可带决策结论）。
func (s *Service) AddResearch(ctx context.Context, cubeID, projectID, userID int64, content, decision string) (contract.Research, error) {
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return contract.Research{}, err
	}
	if p.CubeID != cubeID {
		return contract.Research{}, domain.ErrNotFound
	}
	if content == "" {
		return contract.Research{}, domain.ErrInvalidInput
	}
	ok, err := s.canManage(ctx, cubeID, userID)
	if err != nil {
		return contract.Research{}, err
	}
	if !ok && p.LeaderID != userID {
		return contract.Research{}, domain.ErrNoPermission
	}
	r := infra.Research{ID: int64(ids.New()), ProjectID: projectID, Content: content, Decision: decision, OperatorID: userID}
	if err := s.repo.CreateResearch(ctx, s.pool, r); err != nil {
		return contract.Research{}, err
	}
	return contract.Research{ID: r.ID, ProjectID: projectID, Content: content, Decision: decision,
		OperatorID: userID, CreatedAt: time.Now().UTC()}, nil
}

// ListResearch 研究记录。
func (s *Service) ListResearch(ctx context.Context, cubeID, projectID, userID int64) ([]contract.Research, error) {
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if p.CubeID != cubeID {
		return nil, domain.ErrNotFound
	}
	rows, err := s.repo.ListResearch(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Research, 0, len(rows))
	for _, r := range rows {
		out = append(out, contract.Research{ID: r.ID, ProjectID: r.ProjectID, Content: r.Content,
			Decision: r.Decision, OperatorID: r.OperatorID, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// ---- 任务四态 ----

// CreateTask 新建任务（projectID=0 表示内部工作安排）。
func (s *Service) CreateTask(ctx context.Context, cubeID, userID int64, in contract.TaskInput) (contract.Task, error) {
	if err := domain.ValidateTaskTitle(in.Title); err != nil {
		return contract.Task{}, err
	}
	ok, err := s.canManage(ctx, cubeID, userID)
	if err != nil {
		return contract.Task{}, err
	}
	if !ok {
		return contract.Task{}, domain.ErrNoPermission
	}
	t := infra.Task{ID: int64(ids.New()), CubeID: cubeID, Title: in.Title,
		AssigneeID: in.AssigneeID, ConfirmUserID: in.ConfirmUserID, DueAt: in.DueAt}
	if in.ProjectID > 0 {
		v := in.ProjectID
		t.ProjectID = &v
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.CreateTask(ctx, tx, t); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewTaskAssigned(ids.New().String(), contract.TaskPayload{
			CubeID: cubeID, TaskID: t.ID, ProjectID: in.ProjectID, UserID: in.AssigneeID, OperatorID: userID, Status: 0,
		}))
	})
	if err != nil {
		return contract.Task{}, err
	}
	return toTask(t), nil
}

// ListTasks 任务列表。
func (s *Service) ListTasks(ctx context.Context, cubeID, projectID int64) ([]contract.Task, error) {
	rows, err := s.repo.ListTasks(ctx, cubeID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Task, 0, len(rows))
	for _, t := range rows {
		out = append(out, toTask(t))
	}
	return out, nil
}

// StartTask 开始任务（负责人）。
func (s *Service) StartTask(ctx context.Context, taskID, userID int64) error {
	return s.taskTransition(ctx, taskID, userID, domain.EventStart, domain.TaskDoing, 0)
}

// SubmitTask 提交进度转待确认（负责人）。
func (s *Service) SubmitTask(ctx context.Context, taskID, userID int64, progress int16) error {
	return s.taskTransition(ctx, taskID, userID, domain.EventSubmit, domain.TaskReviewing, progress)
}

// ConfirmTask 确认通过转已完成；驳回须填原因退回进行中。
func (s *Service) ConfirmTask(ctx context.Context, taskID, userID int64, approve bool, reason string) error {
	if !approve && reason == "" {
		return domain.ErrRejectReason
	}
	event := domain.EventApprove
	target := domain.TaskDone
	if !approve {
		event = domain.EventReject
		target = domain.TaskDoing
	}
	return s.taskTransition2(ctx, taskID, userID, event, target, -1, reason)
}

// ReopenTask 重开已完成任务（记操作人与时间）。
func (s *Service) ReopenTask(ctx context.Context, taskID, userID int64) error {
	t, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	ok, err := s.canManage(ctx, t.CubeID, userID)
	if err != nil || !ok {
		return domain.ErrNoPermission
	}
	log, _ := json.Marshal([]map[string]any{{"by": userID, "at": time.Now().UTC()}})
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.ReopenTask(ctx, tx, taskID, userID, log); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewTaskAssigned(ids.New().String(), contract.TaskPayload{
			CubeID: t.CubeID, TaskID: taskID, ProjectID: deref(t.ProjectID), UserID: t.AssigneeID, OperatorID: userID, Status: 1,
		}))
	})
	return err
}

func (s *Service) taskTransition(ctx context.Context, taskID, userID int64, event, targetState string, progress int16) error {
	return s.taskTransition2(ctx, taskID, userID, event, targetState, progress, "")
}

func (s *Service) taskTransition2(ctx context.Context, taskID, userID int64, event, targetState string, progress int16, rejectReason string) error {
	t, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	// 权限：提交需负责人；开始需负责人或管理者；确认需指定确认人或猫王/猫蜜。
	manage, err := s.canManage(ctx, t.CubeID, userID)
	if err != nil {
		return err
	}
	switch event {
	case domain.EventStart, domain.EventSubmit:
		if t.AssigneeID != userID && !manage {
			return domain.ErrNoPermission
		}
	case domain.EventApprove, domain.EventReject:
		if t.ConfirmUserID != 0 && t.ConfirmUserID != userID && !manage {
			return domain.ErrNoPermission
		}
		if t.ConfirmUserID == 0 && !manage {
			return domain.ErrNoPermission
		}
	}
	actor := "assignee"
	if event == domain.EventApprove || event == domain.EventReject {
		actor = "confirmer"
	}
	if _, err := statemachine.NewEngine(nil).Apply(ctx, domain.MachineTask, taskID, taskState(t.Status), event, actor); err != nil {
		return domain.ErrTaskTransition
	}
	newProgress := t.Progress
	if progress >= 0 {
		newProgress = progress
	}
	if targetState == domain.TaskDone {
		newProgress = 100
	}
	target := taskCode(targetState)
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.UpdateTaskStatus(ctx, tx, taskID, target, newProgress, rejectReason); err != nil {
			return err
		}
		ev := contract.NewTaskSubmitted
		if event == domain.EventApprove {
			ev = contract.NewTaskConfirmed
		}
		return s.publish(ctx, tx, ev(ids.New().String(), contract.TaskPayload{
			CubeID: t.CubeID, TaskID: taskID, ProjectID: deref(t.ProjectID), UserID: t.AssigneeID,
			OperatorID: userID, Status: target, Reason: rejectReason,
		}))
	})
	return err
}

// ---- 重点客户报备 ----

// SubmitReport 提交报备（重复仅提示不拦截）。
func (s *Service) SubmitReport(ctx context.Context, cubeID, userID int64, in contract.ReportInput) (contract.Report, bool, error) {
	if in.CustomerName == "" {
		return contract.Report{}, false, domain.ErrInvalidInput
	}
	if err := domain.ValidateNote(in.Note); err != nil {
		return contract.Report{}, false, err
	}
	role, err := s.RoleOf(ctx, cubeID, userID)
	if err != nil {
		return contract.Report{}, false, err
	}
	if role == contract.RoleNone {
		return contract.Report{}, false, domain.ErrNoPermission
	}
	var enc []byte
	if in.ContactPhone != "" {
		if s.enc != nil {
			v, err := s.enc.Encrypt([]byte(in.ContactPhone))
			if err != nil {
				return contract.Report{}, false, err
			}
			enc = v
		} else {
			enc = []byte(in.ContactPhone)
		}
	}
	dup, err := s.repo.CountDuplicateReport(ctx, cubeID, in.CustomerName)
	if err != nil {
		return contract.Report{}, false, err
	}
	r := infra.Report{ID: int64(ids.New()), CubeID: cubeID, ReporterID: userID,
		CustomerName: in.CustomerName, CustomerContactEnc: enc, CustomerType: in.CustomerType,
		RegionCode: in.RegionCode, RelatedItemRef: in.RelatedItemRef, AmountWan: in.AmountWan, Note: in.Note}
	if in.ProjectID > 0 {
		v := in.ProjectID
		r.ProjectID = &v
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.CreateReport(ctx, tx, r); err != nil {
			return err
		}
		return s.publish(ctx, tx, contract.NewReportSubmitted(ids.New().String(), contract.ReportPayload{
			CubeID: cubeID, ReportID: r.ID, ProjectID: in.ProjectID, UserID: userID,
		}))
	})
	if err != nil {
		return contract.Report{}, false, err
	}
	return toReport(r), dup > 0, nil
}

// ListReports 报备列表。
func (s *Service) ListReports(ctx context.Context, cubeID, projectID int64) ([]contract.Report, error) {
	rows, err := s.repo.ListReports(ctx, cubeID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Report, 0, len(rows))
	for _, r := range rows {
		out = append(out, toReport(r))
	}
	return out, nil
}

// ConfirmReport 确认报备。
func (s *Service) ConfirmReport(ctx context.Context, reportID, userID int64) error {
	return s.reportDecision(ctx, reportID, userID, 1, "")
}

// RejectReport 驳回报备（须填原因）。
func (s *Service) RejectReport(ctx context.Context, reportID, userID int64, reason string) error {
	if reason == "" {
		return domain.ErrRejectReason
	}
	return s.reportDecision(ctx, reportID, userID, 2, reason)
}

func (s *Service) reportDecision(ctx context.Context, reportID, userID int64, status int16, reason string) error {
	r, err := s.repo.GetReport(ctx, reportID)
	if err != nil {
		return err
	}
	ok, err := s.canManage(ctx, r.CubeID, userID)
	if err != nil || !ok {
		return domain.ErrNoPermission
	}
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.SetReportStatus(ctx, tx, reportID, status, reason, 0); err != nil {
			return err
		}
		ev := contract.NewReportConfirmed
		if status == 2 {
			ev = contract.NewReportRejected
		}
		return s.publish(ctx, tx, ev(ids.New().String(), contract.ReportPayload{
			CubeID: r.CubeID, ReportID: reportID, ProjectID: deref(r.ProjectID), UserID: r.ReporterID, Reason: reason,
		}))
	})
	if err != nil {
		slog.Error("cube 报备决议失败", "err", err)
	}
	return err
}

// ---- 映射 ----

func deref(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// platformStatus 平台项目状态（平台下架显示「平台已下架」而不删除）。
func (s *Service) platformStatus(ctx context.Context, contentID int64) string {
	if contentID == 0 || s.entities == nil {
		return ""
	}
	info, err := s.entities.Resolve(ctx, entityref.Ref{Type: "content.project", ID: contentID})
	if err != nil {
		// 引用的平台项目对游客不可解析（已下架/不可见）→ 显示「平台已下架」，不删除立方卡片
		return "taken_down"
	}
	switch info.Status {
	case "taken_down", "inactive":
		return "taken_down"
	case "published", "active":
		return "active"
	default:
		return "" // 审核中等：不展示平台状态
	}
}

func toProject(p infra.Project) contract.Project {
	return contract.Project{ID: p.ID, CubeID: p.CubeID, Type: p.Type, ContentProjectID: deref(p.ContentProject),
		Name: p.Name, RelatedRef: p.RelatedRef, RelatedCustomer: p.RelatedCustomer, LeaderID: p.LeaderID,
		Participants: p.Participants, Progress: p.Progress, TodoCount: p.TODO, Status: p.Status,
		SourceRef: p.SourceRef, CreatedAt: p.CreatedAt}
}

func toTask(t infra.Task) contract.Task {
	return contract.Task{ID: t.ID, CubeID: t.CubeID, ProjectID: deref(t.ProjectID), Title: t.Title,
		AssigneeID: t.AssigneeID, Progress: t.Progress, ConfirmUserID: t.ConfirmUserID, DueAt: t.DueAt,
		Status: t.Status, RejectReason: t.RejectReason, ReopenedCount: t.ReopenedCount, CreatedAt: t.CreatedAt}
}

func toReport(r infra.Report) contract.Report {
	return contract.Report{ID: r.ID, CubeID: r.CubeID, ProjectID: deref(r.ProjectID), ReporterID: r.ReporterID,
		CustomerName: r.CustomerName, CustomerType: r.CustomerType, RegionCode: r.RegionCode,
		RelatedItemRef: r.RelatedItemRef, AmountWan: r.AmountWan, Note: r.Note, Status: r.Status,
		RejectReason: r.RejectReason, ConfirmedAt: r.ConfirmedAt, CreatedAt: r.CreatedAt}
}
