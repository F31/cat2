package infra

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Project 立方内项目行。
type Project struct {
	ID             int64
	CubeID         int64
	Type           int16
	ContentProject *int64
	Name           string
	RelatedRef     string
	RelatedCustomer string
	LeaderID       int64
	Participants   []string
	Progress       int16
	TODO           int
	Status         int16
	SourceRef      string
	CreatedAt      time.Time
}

// Task 任务行。
type Task struct {
	ID            int64
	CubeID        int64
	ProjectID     *int64
	Title         string
	AssigneeID    int64
	Progress      int16
	ConfirmUserID int64
	DueAt         *time.Time
	Status        int16
	RejectReason  string
	ReopenedCount int
	CreatedAt     time.Time
}

// Report 报备行。
type Report struct {
	ID             int64
	CubeID         int64
	ProjectID      *int64
	ReporterID     int64
	CustomerName   string
	CustomerContactEnc []byte
	CustomerType   string
	RegionCode     string
	RelatedItemRef string
	AmountWan      int64
	Note           string
	Status         int16
	RejectReason   string
	ConfirmedAt    *time.Time
	CreatedAt      time.Time
}

// Research 研究记录行。
type Research struct {
	ID        int64
	ProjectID int64
	Content   string
	Decision  string
	OperatorID int64
	CreatedAt time.Time
}

// LinkReq 项目引入申请行。
type LinkReq struct {
	ID               int64
	CubeID           int64
	ContentProjectID int64
	InitiatorID      int64
	TargetRole       int16
	Status           int16
	CreatedAt        time.Time
}

const projectCols = `id, cube_id, type, content_project_id, name, COALESCE(related_ref,''), COALESCE(related_customer,''),
	leader_id, participants, progress, todo_count, status, COALESCE(source_ref,''), created_at`

func scanProject(row pgx.Row) (Project, error) {
	var p Project
	var part []byte
	err := row.Scan(&p.ID, &p.CubeID, &p.Type, &p.ContentProject, &p.Name, &p.RelatedRef, &p.RelatedCustomer,
		&p.LeaderID, &part, &p.Progress, &p.TODO, &p.Status, &p.SourceRef, &p.CreatedAt)
	if err != nil {
		return Project{}, err
	}
	_ = json.Unmarshal(part, &p.Participants)
	return p, nil
}

// CreateProject 新建项目。
func (s *Store) CreateProject(ctx context.Context, q Querier, p Project) error {
	part, _ := json.Marshal(p.Participants)
	_, err := q.Exec(ctx, `
		INSERT INTO cub_project (id, cube_id, type, content_project_id, name, related_ref, related_customer,
			leader_id, participants, progress, todo_count, status, source_ref)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,0,0,0,$10)`,
		p.ID, p.CubeID, p.Type, nullIDPtr(p.ContentProject), p.Name, nullStr(p.RelatedRef), nullStr(p.RelatedCustomer),
		p.LeaderID, part, nullStr(p.SourceRef))
	return err
}

func nullIDPtr(v *int64) any {
	if v == nil || *v == 0 {
		return nil
	}
	return *v
}

// GetProject 取项目。
func (s *Store) GetProject(ctx context.Context, id int64) (Project, error) {
	p, err := scanProject(s.pool.QueryRow(ctx, `SELECT `+projectCols+` FROM cub_project WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

// ListProjects 立方内项目（type<=0 表示全部；status<0 表示全部）。
func (s *Store) ListProjects(ctx context.Context, cubeID int64, typ, status int16) ([]Project, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+projectCols+` FROM cub_project
		WHERE cube_id=$1 AND ($2<=0 OR type=$2) AND ($3<0 OR status=$3) ORDER BY created_at DESC`,
		cubeID, typ, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetProjectStatus 变更项目状态。
func (s *Store) SetProjectStatus(ctx context.Context, q Querier, id int64, status int16) error {
	_, err := q.Exec(ctx, `UPDATE cub_project SET status=$2,
		closed_at=CASE WHEN $2::smallint=1 THEN now() ELSE NULL END, updated_at=now() WHERE id=$1`, id, status)
	return err
}

// CreateTask 新建任务。
func (s *Store) CreateTask(ctx context.Context, q Querier, t Task) error {
	_, err := q.Exec(ctx, `
		INSERT INTO cub_task (id, cube_id, project_id, title, assignee_id, progress, confirm_user_id, due_at, status)
		VALUES ($1,$2,$3,$4,$5,0,$6,$7,0)`,
		t.ID, t.CubeID, nullIDPtr(t.ProjectID), t.Title, t.AssigneeID, nullID(t.ConfirmUserID), t.DueAt)
	return err
}

// GetTask 取任务。
func (s *Store) GetTask(ctx context.Context, id int64) (Task, error) {
	var t Task
	err := s.pool.QueryRow(ctx, `
		SELECT id, cube_id, project_id, title, assignee_id, progress, COALESCE(confirm_user_id,0), due_at,
			status, COALESCE(reject_reason,''), reopened_count, created_at
		FROM cub_task WHERE id=$1`, id).
		Scan(&t.ID, &t.CubeID, &t.ProjectID, &t.Title, &t.AssigneeID, &t.Progress, &t.ConfirmUserID,
			&t.DueAt, &t.Status, &t.RejectReason, &t.ReopenedCount, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}

// ListTasks 立方内任务（projectID=0 表示全部）。
func (s *Store) ListTasks(ctx context.Context, cubeID, projectID int64) ([]Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, cube_id, project_id, title, assignee_id, progress, COALESCE(confirm_user_id,0), due_at,
			status, COALESCE(reject_reason,''), reopened_count, created_at
		FROM cub_task WHERE cube_id=$1 AND ($2=0 OR project_id=$2) ORDER BY created_at DESC`, cubeID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.CubeID, &t.ProjectID, &t.Title, &t.AssigneeID, &t.Progress,
			&t.ConfirmUserID, &t.DueAt, &t.Status, &t.RejectReason, &t.ReopenedCount, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateTaskStatus 变更任务状态（提交进度/通过/驳回）。
func (s *Store) UpdateTaskStatus(ctx context.Context, q Querier, id int64, status int16, progress int16, rejectReason string) error {
	_, err := q.Exec(ctx, `UPDATE cub_task SET status=$2, progress=$3, reject_reason=NULLIF($4,''), updated_at=now() WHERE id=$1`,
		id, status, progress, rejectReason)
	return err
}

// ReopenTask 重开已完成任务（记操作人与时间）。
func (s *Store) ReopenTask(ctx context.Context, q Querier, id int64, operatorID int64, logJSON []byte) error {
	_, err := q.Exec(ctx, `UPDATE cub_task SET status=1, reopened_count=reopened_count+1,
		reopen_log = reopen_log || $3::jsonb, updated_at=now() WHERE id=$1 AND status=$2`,
		id, 3, logJSON)
	_ = operatorID
	return err
}

// CreateReport 新建报备。
func (s *Store) CreateReport(ctx context.Context, q Querier, r Report) error {
	_, err := q.Exec(ctx, `
		INSERT INTO cub_report (id, cube_id, project_id, reporter_id, customer_name, customer_contact_enc,
			customer_type, region_code, related_item_ref, amount_wan, note, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,0)`,
		r.ID, r.CubeID, nullIDPtr(r.ProjectID), r.ReporterID, r.CustomerName, r.CustomerContactEnc,
		r.CustomerType, r.RegionCode, nullStr(r.RelatedItemRef), r.AmountWan, nullStr(r.Note))
	return err
}

// CountDuplicateReport 同立方同客户名已存在报备数（重复仅提示）。
func (s *Store) CountDuplicateReport(ctx context.Context, cubeID int64, customerName string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(1) FROM cub_report WHERE cube_id=$1 AND customer_name=$2 AND status<>2`, cubeID, customerName).Scan(&n)
	return n, err
}

// GetReport 取报备。
func (s *Store) GetReport(ctx context.Context, id int64) (Report, error) {
	var r Report
	err := s.pool.QueryRow(ctx, `
		SELECT id, cube_id, project_id, reporter_id, customer_name, COALESCE(customer_type,''), COALESCE(region_code,''),
			COALESCE(related_item_ref,''), COALESCE(amount_wan,0), COALESCE(note,''), status, COALESCE(reject_reason,''),
			confirmed_at, created_at
		FROM cub_report WHERE id=$1`, id).
		Scan(&r.ID, &r.CubeID, &r.ProjectID, &r.ReporterID, &r.CustomerName, &r.CustomerType, &r.RegionCode,
			&r.RelatedItemRef, &r.AmountWan, &r.Note, &r.Status, &r.RejectReason, &r.ConfirmedAt, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	return r, err
}

// ListReports 报备列表（按状态分组由前端处理）。
func (s *Store) ListReports(ctx context.Context, cubeID, projectID int64) ([]Report, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, cube_id, project_id, reporter_id, customer_name, COALESCE(customer_type,''), COALESCE(region_code,''),
			COALESCE(related_item_ref,''), COALESCE(amount_wan,0), COALESCE(note,''), status, COALESCE(reject_reason,''),
			confirmed_at, created_at
		FROM cub_report WHERE cube_id=$1 AND ($2=0 OR project_id=$2) ORDER BY created_at DESC`, cubeID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Report
	for rows.Next() {
		var r Report
		if err := rows.Scan(&r.ID, &r.CubeID, &r.ProjectID, &r.ReporterID, &r.CustomerName, &r.CustomerType,
			&r.RegionCode, &r.RelatedItemRef, &r.AmountWan, &r.Note, &r.Status, &r.RejectReason, &r.ConfirmedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetReportStatus 报备确认/驳回。
func (s *Store) SetReportStatus(ctx context.Context, q Querier, id int64, status int16, rejectReason string, confirmOrg int64) error {
	_, err := q.Exec(ctx, `UPDATE cub_report SET status=$2, reject_reason=NULLIF($3,''),
		confirm_org_id=NULLIF($4,0), confirmed_at=CASE WHEN $2::smallint=1 THEN now() ELSE NULL END WHERE id=$1`,
		id, status, rejectReason, confirmOrg)
	return err
}

// CreateResearch 新增研究记录。
func (s *Store) CreateResearch(ctx context.Context, q Querier, r Research) error {
	_, err := q.Exec(ctx, `INSERT INTO cub_research (id, project_id, content, decision, operator_id)
		VALUES ($1,$2,$3,NULLIF($4,''),$5)`, r.ID, r.ProjectID, r.Content, r.Decision, r.OperatorID)
	return err
}

// ListResearch 研究记录（新→旧）。
func (s *Store) ListResearch(ctx context.Context, projectID int64) ([]Research, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, project_id, content, COALESCE(decision,''), operator_id, created_at
		FROM cub_research WHERE project_id=$1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Research
	for rows.Next() {
		var r Research
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Content, &r.Decision, &r.OperatorID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateLinkReq 新建项目引入申请。
func (s *Store) CreateLinkReq(ctx context.Context, q Querier, l LinkReq) error {
	_, err := q.Exec(ctx, `INSERT INTO cub_project_link_req (id, cube_id, content_project_id, initiator_id, target_role, status)
		VALUES ($1,$2,$3,$4,$5,0)`, l.ID, l.CubeID, l.ContentProjectID, l.InitiatorID, l.TargetRole)
	return err
}

// SetLinkReqStatus 引入申请决议。
func (s *Store) SetLinkReqStatus(ctx context.Context, q Querier, id int64, status int16) error {
	_, err := q.Exec(ctx, `UPDATE cub_project_link_req SET status=$2 WHERE id=$1`, id, status)
	return err
}

// UpdateProjectContentID 回写平台项目 ID（发布到平台）。
func (s *Store) UpdateProjectContentID(ctx context.Context, q Querier, id, contentItemID int64) error {
	_, err := q.Exec(ctx, `UPDATE cub_project SET content_project_id=$2, updated_at=now() WHERE id=$1`, id, contentItemID)
	return err
}

// GetLinkReq 取引入申请。
func (s *Store) GetLinkReq(ctx context.Context, id int64) (LinkReq, error) {
	var l LinkReq
	err := s.pool.QueryRow(ctx, `SELECT id, cube_id, content_project_id, initiator_id, target_role, status, created_at
		FROM cub_project_link_req WHERE id=$1`, id).
		Scan(&l.ID, &l.CubeID, &l.ContentProjectID, &l.InitiatorID, &l.TargetRole, &l.Status, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LinkReq{}, ErrNotFound
	}
	return l, err
}

// ListLinkReqs 引入申请列表（status<0 全部）。
func (s *Store) ListLinkReqs(ctx context.Context, cubeID int64, status int16) ([]LinkReq, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, cube_id, content_project_id, initiator_id, target_role, status, created_at
		FROM cub_project_link_req WHERE cube_id=$1 AND ($2<0 OR status=$2) ORDER BY created_at DESC`, cubeID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LinkReq
	for rows.Next() {
		var l LinkReq
		if err := rows.Scan(&l.ID, &l.CubeID, &l.ContentProjectID, &l.InitiatorID, &l.TargetRole, &l.Status, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
