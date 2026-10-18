package cube

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/obs"
	"catcube/internal/modules/cube/contract"
	tradecontract "catcube/internal/modules/trade/contract"
	"fmt"
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

// ---- 订阅与分摊（§3）----

func (m *Module) Plans(ctx context.Context) ([]contract.Plan, error) { return m.svc.Plans(ctx), nil }

func (m *Module) ChangePlan(ctx context.Context, cubeID, userID int64, level int16, years int) (int64, int64, error) {
	o, err := m.svc.ChangePlan(ctx, cubeID, userID, level, years)
	if err != nil {
		return 0, 0, err
	}
	return o.ID, o.AmountFen, nil
}

func (m *Module) InitiateApportion(ctx context.Context, cubeID, userID int64, level int16, n int, memberIDs []int64) (contract.Apportion, error) {
	g, err := m.svc.InitiateApportion(ctx, cubeID, userID, level, n, memberIDs)
	if err != nil {
		return contract.Apportion{}, err
	}
	return toApportionDTO(cubeID, g.ID, g.PerFen, g.TargetN, g.PledgedCount, g.Deadline, g.Status), nil
}

func (m *Module) PledgeApportion(ctx context.Context, cubeID, groupID, userID int64) (int64, error) {
	o, err := m.svc.PledgeApportion(ctx, cubeID, groupID, userID)
	if err != nil {
		return 0, err
	}
	return o.ID, nil
}

func (m *Module) ExitApportion(ctx context.Context, cubeID, groupID, userID int64) error {
	_, err := m.svc.ExitApportion(ctx, cubeID, groupID, userID)
	return err
}

func (m *Module) ApportionOf(ctx context.Context, groupID int64) (contract.Apportion, error) {
	g, err := m.svc.ApportionOf(ctx, groupID)
	if err != nil {
		return contract.Apportion{}, err
	}
	cubeID, _ := strconv.ParseInt(g.CubePlanRef, 10, 64)
	return toApportionDTO(cubeID, g.ID, g.PerFen, g.TargetN, g.PledgedCount, g.Deadline, g.Status), nil
}

func (m *Module) FulfillOrder(ctx context.Context, bizRef string, snapshot []byte) error {
	return m.svc.Fulfill(ctx, tradecontract.Order{BizType: "cube_plan", BizRef: bizRef, PriceSnapshot: snapshot})
}

// BizType 实现 trade.OrderFulfiller。
func (m *Module) BizType() string { return "cube_plan" }

// handleAdminDetail 平台立方详情：基本信息 + 成员（含昵称）+ 项目。
func (m *Module) handleAdminDetail(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if id == 0 {
		writeErr(w, 1, "参数错误")
		return
	}
	d, err := m.svc.Detail(req.Context(), 0, id)
	if err != nil {
		writeErr(w, 4, "立方不存在")
		return
	}
	members, _ := m.svc.Members(req.Context(), id)
	memberItems := make([]map[string]any, 0, len(members))
	for _, mb := range members {
		nick := ""
		if b, err := m.deps.Identity.UserBrief(req.Context(), mb.UserID); err == nil {
			nick = b.Nickname
		}
		memberItems = append(memberItems, map[string]any{"user_id": strconv.FormatInt(mb.UserID, 10),
			"nickname": nick, "role": mb.Role, "status": mb.Status, "joined_at": mb.JoinedAt.Unix()})
	}
	projects, _ := m.svc.ListProjects(req.Context(), id, 0)
	writeData(w, map[string]any{"cube": d, "members": memberItems, "projects": projects})
}

// handleAdminProjects 平台查看立方研究/承接项目与进度。
func (m *Module) handleAdminProjects(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if id == 0 {
		writeErr(w, 1, "参数错误")
		return
	}
	projects, err := m.svc.ListProjects(req.Context(), id, 0)
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	writeData(w, map[string]any{"items": projects})
}

// handleAdminRemoveMember 平台移除立方成员（不可移除猫王，必填原因）。
func (m *Module) handleAdminRemoveMember(w http.ResponseWriter, req *http.Request) {
	cubeID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	uid, _ := strconv.ParseInt(req.PathValue("uid"), 10, 64)
	var body struct {
		Reason string `json:"reason"`
	}
	if cubeID == 0 || uid == 0 || json.NewDecoder(req.Body).Decode(&body) != nil || strings.TrimSpace(body.Reason) == "" {
		writeErr(w, 1, "参数错误（必须给原因）")
		return
	}
	if err := m.svc.AdminRemoveMember(req.Context(), cubeID, uid, body.Reason); err != nil {
		writeErr(w, 4, "移除失败："+err.Error())
		return
	}
	m.auditCube(req, "cube:admin:member_remove", fmt.Sprintf(`{"type":"cube","id":"%d"}`, cubeID),
		map[string]any{"user_id": strconv.FormatInt(uid, 10), "reason": body.Reason})
	writeData(w, map[string]any{"removed": true})
}

// handleAdminSetLeader 平台调整猫王。
func (m *Module) handleAdminSetLeader(w http.ResponseWriter, req *http.Request) {
	cubeID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		UserID string `json:"user_id"`
	}
	if cubeID == 0 || json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, 1, "参数错误")
		return
	}
	uid, err := strconv.ParseInt(body.UserID, 10, 64)
	if err != nil || uid == 0 {
		writeErr(w, 1, "用户 ID 不合法")
		return
	}
	d, err := m.svc.Detail(req.Context(), 0, cubeID)
	if err != nil {
		writeErr(w, 4, "立方不存在")
		return
	}
	if err := m.svc.Transfer(req.Context(), cubeID, d.LeaderID, uid); err != nil {
		writeErr(w, 4, "调整失败："+err.Error())
		return
	}
	m.auditCube(req, "cube:admin:leader", fmt.Sprintf(`{"type":"cube","id":"%d"}`, cubeID),
		map[string]any{"from": strconv.FormatInt(d.LeaderID, 10), "to": body.UserID})
	writeData(w, map[string]any{"updated": true})
}

// handleAdminSetHoney 平台调整猫蜜。
func (m *Module) handleAdminSetHoney(w http.ResponseWriter, req *http.Request) {
	cubeID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		UserID string `json:"user_id"`
	}
	if cubeID == 0 || json.NewDecoder(req.Body).Decode(&body) != nil {
		writeErr(w, 1, "参数错误")
		return
	}
	uid, err := strconv.ParseInt(body.UserID, 10, 64)
	if err != nil || uid == 0 {
		writeErr(w, 1, "用户 ID 不合法")
		return
	}
	if err := m.svc.AdminSetHoney(req.Context(), cubeID, uid); err != nil {
		writeErr(w, 4, "调整失败："+err.Error())
		return
	}
	m.auditCube(req, "cube:admin:honey", fmt.Sprintf(`{"type":"cube","id":"%d"}`, cubeID),
		map[string]any{"to": body.UserID})
	writeData(w, map[string]any{"updated": true})
}

// handleAdminChats 平台合规查看立方/项目群聊记录（只读，写审计）。
func (m *Module) handleAdminChats(w http.ResponseWriter, req *http.Request) {
	cubeID, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if cubeID == 0 {
		writeErr(w, 1, "参数错误")
		return
	}
	projects, err := m.svc.ListProjects(req.Context(), cubeID, 0)
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	refs := []string{"cube:" + strconv.FormatInt(cubeID, 10)}
	for _, p := range projects {
		refs = append(refs, "cube.project:"+strconv.FormatInt(p.ID, 10))
	}
	if m.deps.Msg == nil {
		writeData(w, map[string]any{"items": []any{}})
		return
	}
	history, err := m.deps.Msg.AdminHistory(req.Context(), refs, 200)
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	items := make([]map[string]any, 0, len(history))
	for _, sess := range history {
		msgs := make([]map[string]any, 0, len(sess.Messages))
		for _, msg := range sess.Messages {
			nick := ""
			if b, err := m.deps.Identity.UserBrief(req.Context(), msg.SenderID); err == nil {
				nick = b.Nickname
			}
			msgs = append(msgs, map[string]any{"id": strconv.FormatInt(msg.ID, 10), "seq": msg.Seq,
				"sender_id": strconv.FormatInt(msg.SenderID, 10), "sender_name": nick,
				"msg_type": msg.MsgType, "content": msg.Content, "created_at": msg.CreatedAt.Unix()})
		}
		items = append(items, map[string]any{"session_id": strconv.FormatInt(sess.SessionID, 10),
			"biz_ref": sess.BizRef, "title": sess.Title, "messages": msgs})
	}
	m.auditCube(req, "cube:admin:chat_view", fmt.Sprintf(`{"type":"cube","id":"%d"}`, cubeID),
		map[string]any{"sessions": len(items)})
	writeData(w, map[string]any{"items": items})
}

// handleAdminBanCube 平台封禁立方（status=1）。
func (m *Module) handleAdminBanCube(w http.ResponseWriter, req *http.Request) {
	m.adminCubeStatus(w, req, 1, "cube:admin:ban")
}

// handleAdminDissolveCube 平台解散立方（status=2）。
func (m *Module) handleAdminDissolveCube(w http.ResponseWriter, req *http.Request) {
	m.adminCubeStatus(w, req, 2, "cube:admin:dissolve")
}

func (m *Module) adminCubeStatus(w http.ResponseWriter, req *http.Request, status int16, action string) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	var body struct {
		Reason string `json:"reason"`
	}
	if id == 0 || json.NewDecoder(req.Body).Decode(&body) != nil || body.Reason == "" {
		writeErr(w, 1, "参数错误（必须给原因）")
		return
	}
	var before int16
	if err := m.deps.Pool.QueryRow(req.Context(), `SELECT status FROM cub_cube WHERE id=$1`, id).Scan(&before); err != nil {
		writeErr(w, 4, "立方不存在")
		return
	}
	if _, err := m.deps.Pool.Exec(req.Context(), `
		UPDATE cub_cube SET status=$2, updated_at=now(), version=version+1 WHERE id=$1`, id, status); err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	if m.deps.Audit != nil {
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin", Action: action,
			TargetRef: fmt.Sprintf(`{"id":"%d","type":"cube"}`, id),
			Detail:    audit.Change{Before: map[string]any{"status": before}, After: map[string]any{"status": status}}.Snapshot(body.Reason),
			Result:    audit.ResultOK, IP: audit.ClientIP(req),
		})
	}
	writeData(w, map[string]any{"cube_id": strconv.FormatInt(id, 10), "status": status})
}

// Fulfill 实现 trade.OrderFulfiller（支付成功后开通订阅）。
func (m *Module) Fulfill(ctx context.Context, order tradecontract.Order) error {
	if err := m.svc.Fulfill(ctx, order); err != nil {
		return err
	}
	obs.Default().Inc("catcube_cube_opened_total")
	return nil
}

func toApportionDTO(cubeID, id, perFen int64, targetN, pledged int, deadline time.Time, status int16) contract.Apportion {
	return contract.Apportion{ID: id, CubeID: cubeID, PerFen: perFen, TargetN: targetN,
		PledgedCount: pledged, Deadline: deadline, Status: status}
}

func (m *Module) handlePlans(w http.ResponseWriter, req *http.Request) {
	writeData(w, map[string]any{"items": m.svc.Plans(req.Context())})
}

func (m *Module) handleChangePlan(w http.ResponseWriter, req *http.Request) {
	cubeID := cubePathID(req, "id")
	var body struct {
		Level int16 `json:"level"`
		Years int   `json:"years"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	o, err := m.svc.ChangePlan(req.Context(), cubeID, userIDFrom(req.Context()), body.Level, body.Years)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	m.auditCube(req, "cube:plan:change", fmt.Sprintf(`{"type":"cube","id":"%d"}`, cubeID),
		map[string]any{"level": body.Level, "years": body.Years, "order_id": strconv.FormatInt(o.ID, 10),
			"amount_fen": o.AmountFen})
	writeData(w, map[string]any{"order_id": strconv.FormatInt(o.ID, 10), "amount_fen": o.AmountFen})
}

func (m *Module) handleInitiateApportion(w http.ResponseWriter, req *http.Request) {
	cubeID := cubePathID(req, "id")
	var body struct {
		Level     int16    `json:"level"`
		N         int      `json:"n"`
		MemberIDs []string `json:"member_ids"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	members := make([]int64, 0, len(body.MemberIDs))
	for _, s := range body.MemberIDs {
		if v, e := strconv.ParseInt(s, 10, 64); e == nil {
			members = append(members, v)
		}
	}
	ap, err := m.InitiateApportion(req.Context(), cubeID, userIDFrom(req.Context()), body.Level, body.N, members)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, ap)
}

func (m *Module) handlePledgeApportion(w http.ResponseWriter, req *http.Request) {
	cubeID := cubePathID(req, "id")
	var body struct {
		GroupID int64 `json:"group_id,string"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.GroupID == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	orderID, err := m.PledgeApportion(req.Context(), cubeID, body.GroupID, userIDFrom(req.Context()))
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"order_id": strconv.FormatInt(orderID, 10)})
}

func (m *Module) handleExitApportion(w http.ResponseWriter, req *http.Request) {
	cubeID := cubePathID(req, "id")
	var body struct {
		GroupID int64 `json:"group_id,string"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.GroupID == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	if err := m.ExitApportion(req.Context(), cubeID, body.GroupID, userIDFrom(req.Context())); err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"exited": true})
}

func (m *Module) handleApportionDetail(w http.ResponseWriter, req *http.Request) {
	groupID := cubePathID(req, "groupID")
	ap, err := m.ApportionOf(req.Context(), groupID)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, ap)
}

// ---- 项目关联（§4）----

func (m *Module) RequestLink(ctx context.Context, cubeID, userID, contentItemID int64) (bool, int64, int64, error) {
	return m.svc.RequestLink(ctx, cubeID, userID, contentItemID)
}
func (m *Module) DecideLink(ctx context.Context, cubeID, userID, reqID int64, approve bool) (int64, error) {
	return m.svc.DecideLink(ctx, cubeID, userID, reqID, approve)
}
func (m *Module) LinkRequests(ctx context.Context, cubeID int64) ([]contract.LinkReq, error) {
	return m.svc.LinkRequests(ctx, cubeID)
}
func (m *Module) CreateProjectFromRef(ctx context.Context, cubeID, userID int64, sourceRef, title, body string) (int64, error) {
	return m.svc.CreateProjectFromRef(ctx, cubeID, userID, sourceRef, title, body)
}
func (m *Module) PublishToPlatform(ctx context.Context, cubeID, projectID, userID int64) (int64, error) {
	return m.svc.PublishToPlatform(ctx, cubeID, projectID, userID)
}

func (m *Module) handleRequestLink(w http.ResponseWriter, req *http.Request) {
	cubeID := cubePathID(req, "id")
	var body struct {
		ContentProjectID int64 `json:"content_project_id,string"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.ContentProjectID == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	direct, projectID, reqID, err := m.svc.RequestLink(req.Context(), cubeID, userIDFrom(req.Context()), body.ContentProjectID)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"direct": direct, "project_id": strconv.FormatInt(projectID, 10), "request_id": strconv.FormatInt(reqID, 10)})
}

func (m *Module) handleLinkRequests(w http.ResponseWriter, req *http.Request) {
	items, err := m.svc.LinkRequests(req.Context(), cubePathID(req, "id"))
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleDecideLink(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Approve bool `json:"approve"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	projectID, err := m.svc.DecideLink(req.Context(), cubePathID(req, "id"), userIDFrom(req.Context()), cubePathID(req, "reqID"), body.Approve)
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"project_id": strconv.FormatInt(projectID, 10), "approved": body.Approve})
}

func (m *Module) handlePublishToPlatform(w http.ResponseWriter, req *http.Request) {
	itemID, err := m.svc.PublishToPlatform(req.Context(), cubePathID(req, "id"), cubePathID(req, "pid"), userIDFrom(req.Context()))
	if err != nil {
		writeCubeErr(w, err)
		return
	}
	writeData(w, map[string]any{"content_project_id": strconv.FormatInt(itemID, 10)})
}
