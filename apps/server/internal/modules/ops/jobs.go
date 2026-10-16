package ops

import (
	"net/http"
	"sort"

	"catcube/internal/kernel/audit"
)

// handleJobs 列出可手动重跑的任务（代操作清单：死信重放/任务重跑，p4 §4.4）。
func (m *Module) handleJobs(w http.ResponseWriter, req *http.Request) {
	if m.deps.Runner == nil {
		writeData(w, map[string]any{"items": []any{}})
		return
	}
	names := make([]string, 0)
	for _, j := range m.deps.Runner.Jobs() {
		names = append(names, j.Name)
	}
	sort.Strings(names)
	writeData(w, map[string]any{"items": names})
}

// handleRunJob 手动重跑指定任务（立即执行一次；审计留痕）。
func (m *Module) handleRunJob(w http.ResponseWriter, req *http.Request) {
	name := req.PathValue("name")
	if m.deps.Runner == nil || name == "" {
		writeErr(w, 1, "参数错误")
		return
	}
	found, err := m.deps.Runner.RunOnce(req.Context(), name)
	if !found {
		writeErr(w, 4, "任务不存在")
		return
	}
	uid := userIDFrom(req.Context())
	res := audit.ResultOK
	if err != nil {
		res = audit.ResultFailed
	}
	if m.deps.Audit != nil {
		_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: "ops:job:run",
			TargetRef: `{"id":"` + name + `","type":"job"}`,
			Detail: audit.Change{Before: map[string]any{"state": "idle"},
				After: map[string]any{"state": "triggered", "error": errText(err)}}.Snapshot("手动重跑任务"),
			Result: res, IP: audit.ClientIP(req),
		})
	}
	if err != nil {
		writeErr(w, 9, "任务执行失败: "+err.Error())
		return
	}
	writeData(w, map[string]any{"ran": true, "job": name})
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
