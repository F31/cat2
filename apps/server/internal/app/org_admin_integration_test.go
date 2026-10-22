//go:build integration

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"
	"time"
)

// 运营管理 → 组织管理：新增组织、名片、认证提交/审批、成员辅助加入/移除、列表聚合。
func TestIntegrationAdminOrgManagement(t *testing.T) {
	h := newTestApp(t)
	_, admTok := loginAdmin(t, h)
	phone := fmt.Sprintf("1370000%04d", time.Now().UnixNano()%10000)

	// 1) 后台新增组织账号（六类主体之一），补负责人姓名/证件号/区域
	_, env := h.do(http.MethodPost, "/api/admin/v1/users", admTok, map[string]any{
		"nickname": "集成测试组织", "phone": phone, "identity_type": 2,
		"real_name": "组织负责人", "id_card_no": "310101199001010033", "region_code": "310000",
	})
	if env.Code != 0 {
		t.Fatalf("新增组织失败: %+v", env)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(env.Data, &created)
	if created.ID == "" {
		t.Fatal("新增组织缺少 id")
	}

	// 2) 后台维护组织名片（六类主题：企业）
	if s, e := h.do(http.MethodPut, "/api/admin/v1/users/"+created.ID+"/card", admTok, map[string]any{
		"org_type_code": "company", "name": "集成测试组织", "region_code": "310000",
		"industry_codes": []string{"A01"}, "need_codes": []string{"N1"}, "offer_codes": []string{"O1"},
		"core_strength": "测试优势", "intro": "测试组织简介",
	}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("维护组织名片失败: %d %+v", s, e)
	}

	// 3) 平台提交企业认证（资质材料 ID）
	_, env = h.do(http.MethodPost, "/api/admin/v1/orgs/"+created.ID+"/certifications", admTok, map[string]any{
		"cert_type": 4, "agreed": true, "material_file_ids": []int{1, 2},
		"extra": map[string]string{"org_code": "91310000TEST000001", "legal_name": "组织负责人"},
	})
	if env.Code != 0 {
		t.Fatalf("提交组织认证失败: %+v", env)
	}
	var cert struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(env.Data, &cert)
	if cert.ID == "" {
		t.Fatal("认证缺少 id")
	}

	// 4) 认证审批（超级管理员具备认证审核权限）
	if s, e := h.do(http.MethodPost, "/api/admin/v1/certs/"+cert.ID+"/review", admTok, map[string]any{"pass": true}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("组织认证审批失败: %d %+v", s, e)
	}

	// 5) 成员实名后，管理端辅助加入组织
	member, memberTok := loginUser(t, h)
	if _, e := h.do(http.MethodPost, "/api/mp/v1/certs", memberTok, map[string]any{
		"cert_type": 1, "real_name": "组织成员", "id_card_no": "310101199001010044", "agreed": true,
	}); e.Code != 0 {
		t.Fatalf("成员实名失败: %+v", e)
	}
	if s, e := h.do(http.MethodPost, "/api/admin/v1/orgs/"+created.ID+"/members", admTok, map[string]any{
		"user_id": fmt.Sprintf("%d", member), "role": "合作经理",
	}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("辅助加入组织失败: %d %+v", s, e)
	}

	// 6) 成员列表与组织列表聚合
	_, env = h.do(http.MethodGet, "/api/admin/v1/orgs/"+created.ID+"/members", admTok, nil)
	var members struct {
		Items []struct {
			UserID string `json:"user_id"`
			Status int16  `json:"status"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &members)
	found := false
	for _, m := range members.Items {
		if m.UserID == fmt.Sprintf("%d", member) && m.Status == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("辅助加入后成员应为已核验: %+v", members.Items)
	}
	_, env = h.do(http.MethodGet, "/api/admin/v1/orgs?q=集成测试组织", admTok, nil)
	var orgs struct {
		Items []struct {
			ID          string `json:"id"`
			Category    string `json:"category"`
			CertStatus  int16  `json:"cert_status"`
			MemberCount int    `json:"member_count"`
			AdminCount  int    `json:"admin_count"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &orgs)
	if len(orgs.Items) == 0 || orgs.Items[0].ID != created.ID || orgs.Items[0].Category != "company" ||
		orgs.Items[0].CertStatus != 1 || orgs.Items[0].MemberCount != 1 || orgs.Items[0].AdminCount == 0 {
		t.Fatalf("组织列表聚合不符: %+v", orgs.Items)
	}

	// 7) 移除成员
	if s, e := h.do(http.MethodDelete, "/api/admin/v1/orgs/"+created.ID+"/members/"+fmt.Sprintf("%d", member), admTok, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("移除组织成员失败: %d %+v", s, e)
	}
}

// 后台资质材料 multipart 上传 → 本地落库 → 受保护读取。
func TestIntegrationAdminMediaUpload(t *testing.T) {
	h := newTestApp(t)
	_, admTok := loginAdmin(t, h)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="cert.png"`)
	header.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("\x89PNG\r\n\x1a\nfake-image-bytes")
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/admin/v1/media/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Admin-Token", admTok)
	res, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var uploaded struct {
		Code int `json:"code"`
		Data struct {
			FileID string `json:"file_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&uploaded); err != nil || uploaded.Code != 0 || uploaded.Data.FileID == "" {
		t.Fatalf("上传失败: %+v err=%v", uploaded, err)
	}
	_, env := h.do(http.MethodGet, "/api/admin/v1/media/"+uploaded.Data.FileID, admTok, nil)
	var meta struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(env.Data, &meta)
	want := "/api/admin/v1/media/" + uploaded.Data.FileID + "/raw"
	if meta.URL != want {
		t.Fatalf("本地材料地址不符: %q want %q", meta.URL, want)
	}
	req, _ = http.NewRequest(http.MethodGet, h.srv.URL+meta.URL, nil)
	req.Header.Set("X-Admin-Token", admTok)
	raw, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Body.Close()
	got, _ := io.ReadAll(raw.Body)
	if !bytes.Equal(got, payload) {
		t.Fatalf("材料内容不一致: %q", string(got))
	}
}
