//go:build integration

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"testing"

	"catcube/internal/kernel/config"
)

func uploadUserMedia(t *testing.T, h *testApp, tok string) int64 {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="id.png"`)
	header.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("\x89PNG\r\n\x1a\nfake")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/mp/v1/media/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var env struct {
		Code int `json:"code"`
		Data struct {
			FileID string `json:"file_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Code != 0 || env.Data.FileID == "" {
		t.Fatalf("用户材料上传失败: %s err=%v", string(raw), err)
	}
	id, err := strconv.ParseInt(env.Data.FileID, 10, 64)
	if err != nil || id == 0 {
		t.Fatalf("文件ID非法: %q", env.Data.FileID)
	}
	return id
}

// 实名人工审核：身份证正反面 + 本地严格校验 + 平台审核通过/驳回。
func TestIntegrationRealnameManualReview(t *testing.T) {
	h := newTestAppCfg(t, func(c *config.Config) { c.RealnameManualReview = true })
	_, admTok := loginAdmin(t, h)
	uid, tok := loginUser(t, h)
	ctx := context.Background()

	// 本地校验失败：校验位错误
	if s, e := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
		"material_file_ids": []int{1, 2},
	}); s != http.StatusBadRequest || e.Code != 100001 {
		t.Fatalf("错误校验位应 400/100001: %d %+v", s, e)
	}
	// 材料不足
	if s, e := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070011", "agreed": true,
		"material_file_ids": []int{1},
	}); s != http.StatusBadRequest || e.Code != 100001 {
		t.Fatalf("材料不足应 400/100001: %d %+v", s, e)
	}

	// 上传身份证正反面（走小程序媒体上传）
	id1 := uploadUserMedia(t, h, tok)
	id2 := uploadUserMedia(t, h, tok)

	// 提交待审
	_, env := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070011", "agreed": true,
		"material_file_ids": []int64{id1, id2},
	})
	var cert struct {
		ID     string `json:"id"`
		Status int16  `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &cert)
	if cert.ID == "" || cert.Status != 0 {
		t.Fatalf("人工审核应进入待审: %+v", env)
	}
	var rs int16
	_ = h.app.Pool.QueryRow(ctx, `SELECT realname_status FROM idn_user WHERE id=$1`, uid).Scan(&rs)
	if rs != 0 {
		t.Fatalf("待审期间不应实名，实际 %d", rs)
	}
	// 重复提交冲突
	if s, e := h.do(http.MethodPost, "/api/mp/v1/certs", tok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070011", "agreed": true,
		"material_file_ids": []int64{id1, id2},
	}); s != http.StatusConflict || e.Code != 100010 {
		t.Fatalf("重复提交应 409/100010: %d %+v", s, e)
	}

	// 审核详情：姓名/证件号/正反面材料可读
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/admin/v1/certs/%s/realname", cert.ID), admTok, nil)
	var detail struct {
		RealName        string  `json:"real_name"`
		IDCardNo        string  `json:"id_card_no"`
		MaterialFileIDs []int64 `json:"material_file_ids"`
	}
	_ = json.Unmarshal(env.Data, &detail)
	if detail.RealName != "张三" || detail.IDCardNo != "110101199003070011" || len(detail.MaterialFileIDs) != 2 {
		t.Fatalf("实名审核详情不符: %+v", detail)
	}

	// 平台审核通过 → identity 回写实名 + Badge
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/certs/%s/review", cert.ID), admTok, map[string]any{"pass": true}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("审核通过失败: %d %+v", s, e)
	}
	_ = h.app.Pool.QueryRow(ctx, `SELECT realname_status FROM idn_user WHERE id=$1`, uid).Scan(&rs)
	if rs != 1 {
		t.Fatalf("审核通过后应已实名，实际 %d", rs)
	}
	var badgeN int
	_ = h.app.Pool.QueryRow(ctx, `SELECT count(1) FROM vfy_badge_grant WHERE subject_type='user' AND subject_id=$1 AND badge_code='realname' AND revoked_at IS NULL`, uid).Scan(&badgeN)
	if badgeN == 0 {
		t.Fatal("审核通过应授予实名 Badge")
	}

	// 另一用户驳回 → identity 失败
	uid2, tok2 := loginUser(t, h)
	id3 := uploadUserMedia(t, h, tok2)
	id4 := uploadUserMedia(t, h, tok2)
	_, env = h.do(http.MethodPost, "/api/mp/v1/certs", tok2, map[string]any{
		"cert_type": 1, "real_name": "李四", "id_card_no": "11010119900307002X", "agreed": true,
		"material_file_ids": []int64{id3, id4},
	})
	var cert2 struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(env.Data, &cert2)
	if cert2.ID == "" {
		t.Fatalf("第二用户提交失败: %+v", env)
	}
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/certs/%s/review", cert2.ID), admTok, map[string]any{"pass": false, "reason": "证件不清晰"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("驳回失败: %d %+v", s, e)
	}
	_ = h.app.Pool.QueryRow(ctx, `SELECT realname_status FROM idn_user WHERE id=$1`, uid2).Scan(&rs)
	if rs != 2 {
		t.Fatalf("驳回后应为核验失败，实际 %d", rs)
	}
}
