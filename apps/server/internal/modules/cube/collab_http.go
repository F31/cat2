package cube

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"catcube/internal/modules/cube/contract"
)

// ---- contract.Facade 委托（§2）----

func (m *Module) CreateProject(ctx context.Context, cubeID, userID int64, in contract.ProjectInput) (contract.Project, error) {
	return m.svc.CreateProject(ctx, cubeID, userID, in)
}
func (m *Module) ListProjects(ctx context.Context, cubeID int64, typ int16) ([]contract.Project, error) {
	return m.svc.ListProjects(ctx, cubeID, typ)
}
func (m *Module) CloseProject(ctx context.Context, cubeID, projectID, userID int64) error {
	return m.svc.CloseProject(ctx, cubeID, projectID, userID)
}
func (m *Module) ReopenProject(ctx context.Context, cubeID, projectID, userID int64) error {
	return m.svc.ReopenProject(ctx, cubeID, projectID, userID)
}
func (m *Module) AddResearch(ctx context.Context, cubeID, projectID, userID int64, content, decision string) (contract.Research, error) {
	return m.svc.AddResearch(ctx, cubeID, projectID, userID, content, decision)
}
func (m *Module) ListResearch(ctx context.Context, cubeID, projectID, userID int64) ([]contract.Research, error) {
	return m.svc.ListResearch(ctx, cubeID, projectID, userID)
}
func (m *Module) CreateTask(ctx context.Context, cubeID, userID int64, in contract.TaskInput) (contract.Task, error) {
	return m.svc.CreateTask(ctx, cubeID, userID, in)
}
func (m *Module) ListTasks(ctx context.Context, cubeID, projectID int64) ([]contract.Task, error) {
	return m.svc.ListTasks(ctx, cubeID, projectID)
}
func (m *Module) StartTask(ctx context.Context, taskID, userID int64) error {
	return m.svc.StartTask(ctx, taskID, userID)
}
func (m *Module) SubmitTask(ctx context.Context, taskID, userID int64, progress int16) error {
	return m.svc.SubmitTask(ctx, taskID, userID, progress)
}
func (m *Module) ConfirmTask(ctx context.Context, taskID, userID int64, approve bool, reason string) error {
	return m.svc.ConfirmTask(ctx, taskID, userID, approve, reason)
}
func (m *Module) ReopenTask(ctx context.Context, taskID, userID int64) error {
	return m.svc.ReopenTask(ctx, taskID, userID)
}
func (m *Module) SubmitReport(ctx context.Context, cubeID, userID int64, in contract.ReportInput) (contract.Report, bool, error) {
	return m.svc.SubmitReport(ctx, cubeID, userID, in)
}
func (m *Module) ListReports(ctx context.Context, cubeID, projectID int64) ([]contract.Report, error) {
	return m.svc.ListReports(ctx, cubeID, projectID)
}
func (m *Module) ConfirmReport(ctx context.Context, reportID, userID int64) error {
	return m.svc.ConfirmReport(ctx, reportID, userID)
}
func (m *Module) RejectReport(ctx context.Context, reportID, userID int64, reason string) error {
	return m.svc.RejectReport(ctx, reportID, userID, reason)
}

// ---- HTTP（§2）----

func cubePathID(req *http.Request, name string) int64 {
	id, _ := pathID(req, name)
	return id
}

func (m *Module) handleCreateProject(w http.ResponseWriter, req *http.Request) {
	cubeID := cubePathID(req, "id")
	var body struct {
		Type             int16    `json:"type"`
		Name             string   `json:"name"`
		RelatedRef       string   `json:"related_ref"`
		RelatedCustomer  string   `json:"related_customer"`
		Participants     []string `json:"participants"`
		ContentProjectID int64    `json:"content_project_id,string"`
		SourceRef        string   `json:"source_ref"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	p, err := m.svc.CreateProject(req.Context(), cubeID, userIDFrom(req.Context()), contract.ProjectInput{
		Type: body.Type, Name: body.Name, RelatedRef: body.RelatedRef, RelatedCustomer: body.RelatedCustomer,
		Participants: body.Participants, ContentProjectID: body.ContentProjectID, SourceRef: body.SourceRef,
	})
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, p)
}

func (m *Module) handleListProjects(w http.ResponseWriter, req *http.Request) {
	cubeID := cubePathID(req, "id")
	typ := int16(0)
	if v := req.URL.Query().Get("type"); v != "" {
		typ = int16(atof(v))
	}
	items, err := m.svc.ListProjects(req.Context(), cubeID, typ)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleCloseProject(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.CloseProject(req.Context(), cubePathID(req, "id"), cubePathID(req, "pid"), userIDFrom(req.Context())); err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"closed": true})
}

func (m *Module) handleReopenProject(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.ReopenProject(req.Context(), cubePathID(req, "id"), cubePathID(req, "pid"), userIDFrom(req.Context())); err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"reopened": true})
}

func (m *Module) handleListResearch(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.ListResearch(req.Context(), cubePathID(req, "id"), cubePathID(req, "pid"), userIDFrom(req.Context()))
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleAddResearch(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Content  string `json:"content"`
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	r, err := m.svc.AddResearch(req.Context(), cubePathID(req, "id"), cubePathID(req, "pid"), userIDFrom(req.Context()), body.Content, body.Decision)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, r)
}

func (m *Module) handleCreateTask(w http.ResponseWriter, req *http.Request) {
	var body struct {
		ProjectID     int64  `json:"project_id,string"`
		Title         string `json:"title"`
		AssigneeID    int64  `json:"assignee_id,string"`
		ConfirmUserID int64  `json:"confirm_user_id,string"`
		DueAt         string `json:"due_at"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	var due *time.Time
	if body.DueAt != "" {
		if t, err := time.Parse(time.RFC3339, body.DueAt); err == nil {
			due = &t
		}
	}
	task, err := m.svc.CreateTask(req.Context(), cubePathID(req, "id"), userIDFrom(req.Context()), contract.TaskInput{
		ProjectID: body.ProjectID, Title: body.Title, AssigneeID: body.AssigneeID,
		ConfirmUserID: body.ConfirmUserID, DueAt: due,
	})
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, task)
}

func (m *Module) handleListTasks(w http.ResponseWriter, req *http.Request) {
	projectID := int64(0)
	if v := req.URL.Query().Get("project_id"); v != "" {
		projectID = int64(atof(v))
	}
	items, err := m.svc.ListTasks(req.Context(), cubePathID(req, "id"), projectID)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) taskAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		taskID := cubePathID(req, "tid")
		userID := userIDFrom(req.Context())
		switch action {
		case "start":
			if err := m.svc.StartTask(req.Context(), taskID, userID); err != nil {
				writeCubeErr(w, err)
				return
			}
			writeData(w, map[string]any{"status": 1})
		case "submit":
			var body struct {
				Progress int16 `json:"progress"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if err := m.svc.SubmitTask(req.Context(), taskID, userID, body.Progress); err != nil {
				writeCubeErr(w, err)
				return
			}
			writeData(w, map[string]any{"status": 2})
		case "confirm":
			var body struct {
				Approve bool   `json:"approve"`
				Reason  string `json:"reason"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if err := m.svc.ConfirmTask(req.Context(), taskID, userID, body.Approve, body.Reason); err != nil {
				writeCubeErr(w, err)
				return
			}
			writeData(w, map[string]any{"confirmed": body.Approve})
		case "reopen":
			if err := m.svc.ReopenTask(req.Context(), taskID, userID); err != nil {
				writeCubeErr(w, err)
				return
			}
			writeData(w, map[string]any{"reopened": true})
		default:
			writeErr(w, codeParamInvalid, "参数错误")
		}
	}
}

func (m *Module) handleSubmitReport(w http.ResponseWriter, req *http.Request) {
	var body struct {
		ProjectID      int64  `json:"project_id,string"`
		CustomerName   string `json:"customer_name"`
		ContactPhone   string `json:"contact_phone"`
		CustomerType   string `json:"customer_type"`
		RegionCode     string `json:"region_code"`
		RelatedItemRef string `json:"related_item_ref"`
		AmountWan      int64  `json:"amount_wan"`
		Note           string `json:"note"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	r, dup, err := m.svc.SubmitReport(req.Context(), cubePathID(req, "id"), userIDFrom(req.Context()), contract.ReportInput{
		ProjectID: body.ProjectID, CustomerName: body.CustomerName, ContactPhone: body.ContactPhone,
		CustomerType: body.CustomerType, RegionCode: body.RegionCode, RelatedItemRef: body.RelatedItemRef,
		AmountWan: body.AmountWan, Note: body.Note,
	})
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"report": r, "duplicate": dup})
}

func (m *Module) handleListReports(w http.ResponseWriter, req *http.Request) {
	projectID := int64(0)
	if v := req.URL.Query().Get("project_id"); v != "" {
		projectID = int64(atof(v))
	}
	items, err := m.svc.ListReports(req.Context(), cubePathID(req, "id"), projectID)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleConfirmReport(w http.ResponseWriter, req *http.Request) {
	if err := m.svc.ConfirmReport(req.Context(), cubePathID(req, "rid"), userIDFrom(req.Context())); err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"confirmed": true})
}

func (m *Module) handleRejectReport(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	if err := m.svc.RejectReport(req.Context(), cubePathID(req, "rid"), userIDFrom(req.Context()), body.Reason); err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"rejected": true})
}

func atof(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
