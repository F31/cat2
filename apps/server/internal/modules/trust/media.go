package trust

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"catcube/internal/modules/trust/contract"
	"catcube/pkg/ids"
)

// mediaCallback 微信 media_check_async 回调（字段以官方文档为准，宽松解析）。
type mediaCallback struct {
	TraceID string `json:"trace_id"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	Result  struct {
		Suggest string `json:"suggest"`
		Label   int    `json:"label"`
	} `json:"result"`
	Detail []struct {
		Label int `json:"label"`
	} `json:"detail"`
}

// handleMediaCallback 接收图片异步机检结论。
// 通过 trace_id 找到提交时的 subject；同主体全部图片通过才放行，任一拒绝即驳回。
func (m *Module) handleMediaCallback(w http.ResponseWriter, r *http.Request) {
	if m.deps.CallbackToken != "" && r.URL.Query().Get("token") != m.deps.CallbackToken {
		http.Error(w, `{"errcode":403,"errmsg":"forbidden"}`, http.StatusForbidden)
		return
	}
	var cb mediaCallback
	if err := json.NewDecoder(r.Body).Decode(&cb); err != nil || strings.TrimSpace(cb.TraceID) == "" {
		http.Error(w, `{"errcode":400,"errmsg":"bad request"}`, http.StatusBadRequest)
		return
	}
	if err := m.handleMediaResult(r.Context(), cb); err != nil {
		http.Error(w, `{"errcode":500,"errmsg":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
}

func (m *Module) handleMediaResult(ctx context.Context, cb mediaCallback) error {
	var subjectType string
	var subjectID int64
	// 只处理待回调记录；重复/未知 trace 直接确认，避免微信重试风暴。
	err := m.deps.Pool.QueryRow(ctx, `
		UPDATE trs_media_check SET status=$2, result=$3::jsonb, updated_at=now(), checked_at=now()
		WHERE trace_id=$1 AND status=0
		RETURNING subject_type, subject_id`,
		cb.TraceID, map[bool]int16{true: 1, false: 2}[mediaPass(cb)], mustJSON(cb)).Scan(&subjectType, &subjectID)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil
		}
		return err
	}
	if !mediaPass(cb) {
		// 同主体其它待回调图片取消，最终结论以拒绝为准。
		_, _ = m.deps.Pool.Exec(ctx, `UPDATE trs_media_check SET status=3, updated_at=now()
			WHERE subject_type=$1 AND subject_id=$2 AND status=0`, subjectType, subjectID)
		_ = m.createTask(ctx, contract.SourceMachine, subjectType, subjectID, 0, 2, "图片机检命中")
		_ = m.applyVerdict(ctx, subjectType, subjectID, false, "media_reject")
		_ = m.emit(ctx, contract.NewModerationRejected(ids.New().String(), contract.ModerationPayload{
			SubjectType: subjectType, SubjectID: subjectID, ReasonCode: "media_reject",
		}))
		return nil
	}
	var pending, rejected int
	if err := m.deps.Pool.QueryRow(ctx, `
		SELECT count(1) FILTER (WHERE status=0), count(1) FILTER (WHERE status=2)
		FROM trs_media_check WHERE subject_type=$1 AND subject_id=$2`, subjectType, subjectID).
		Scan(&pending, &rejected); err != nil {
		return err
	}
	if pending == 0 && rejected == 0 {
		_ = m.applyVerdict(ctx, subjectType, subjectID, true, "")
		_ = m.emit(ctx, contract.NewModerationPassed(ids.New().String(), contract.ModerationPayload{
			SubjectType: subjectType, SubjectID: subjectID,
		}))
	}
	return nil
}

func (m *Module) applyVerdict(ctx context.Context, subjectType string, subjectID int64, pass bool, reason string) error {
	if p := m.provider(subjectType); p != nil {
		return p.ApplyVerdict(ctx, subjectID, pass, reason)
	}
	return nil
}

func mediaPass(cb mediaCallback) bool {
	return cb.ErrCode == 0 && strings.EqualFold(strings.TrimSpace(cb.Result.Suggest), "pass")
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
