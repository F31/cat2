//go:build integration

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// PRD §6.5/§11.4：圈子批量封禁/归档/解封/恢复 + 逐圈审计 + 前台隐藏。
func TestIntegrationCircleBatchAdmin(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	_, admTok := loginAdmin(t, h)
	uid, userTok := loginUser(t, h)
	id := time.Now().UnixNano()
	if _, err := h.app.Pool.Exec(ctx, `
		INSERT INTO cir_circle (id, name, owner_user_id, status) VALUES ($1,'批量测试圈',$2,0)`, id, uid); err != nil {
		t.Fatal(err)
	}

	batch := func(action string, ids []string) (int, envelope) {
		return h.do(http.MethodPost, "/api/admin/v1/circles/batch", admTok,
			map[string]any{"action": action, "ids": ids, "reason": "违规处置"})
	}
	var status, prev int16
	var name string
	_, env := batch("ban", []string{fmt.Sprintf("%d", id)})
	if env.Code != 0 {
		t.Fatalf("批量封禁失败: %+v", env)
	}
	_ = h.app.Pool.QueryRow(ctx, `SELECT status, COALESCE(prev_status,-1), name FROM cir_circle WHERE id=$1`, id).Scan(&status, &prev, &name)
	if status != 3 || prev != 0 {
		t.Fatalf("封禁状态不符: status=%d prev=%d env=%+v", status, prev, env)
	}
	if s, e := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/circles/%d", id), "", nil); s != http.StatusNotFound && e.Code != 140001 {
		t.Fatalf("封禁圈前台应不可见: %d %+v", s, e)
	}
	// 审计逐圈写入
	_, env = h.do(http.MethodGet, "/api/admin/v1/audits?action=circle:admin:ban&limit=20", admTok, nil)
	var audits struct {
		Items []struct {
			Action    string `json:"action"`
			TargetRef string `json:"target_ref"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &audits)
	found := false
	for _, a := range audits.Items {
		if a.Action == "circle:admin:ban" {
			found = true
		}
	}
	if !found {
		t.Fatal("批量封禁未写审计")
	}

	// 解封恢复原状态
	if _, env = batch("unban", []string{fmt.Sprintf("%d", id)}); env.Code != 0 {
		t.Fatalf("解封失败: %+v", env)
	}
	_ = h.app.Pool.QueryRow(ctx, `SELECT status FROM cir_circle WHERE id=$1`, id).Scan(&status)
	if status != 0 {
		t.Fatalf("解封应回进行中: %d", status)
	}
	// 归档/恢复
	if _, env = batch("archive", []string{fmt.Sprintf("%d", id)}); env.Code != 0 {
		t.Fatalf("归档失败: %+v", env)
	}
	_ = h.app.Pool.QueryRow(ctx, `SELECT status FROM cir_circle WHERE id=$1`, id).Scan(&status)
	if status != 4 {
		t.Fatalf("归档状态应为4: %d", status)
	}
	if _, env = batch("restore", []string{fmt.Sprintf("%d", id)}); env.Code != 0 {
		t.Fatalf("恢复失败: %+v", env)
	}
	_ = h.app.Pool.QueryRow(ctx, `SELECT status FROM cir_circle WHERE id=$1`, id).Scan(&status)
	if status != 0 {
		t.Fatalf("恢复状态应为0: %d", status)
	}
	// 部分失败逐条返回
	_, env = batch("ban", []string{fmt.Sprintf("%d", id), "999999999"})
	var res struct {
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	}
	_ = json.Unmarshal(env.Data, &res)
	if res.Succeeded != 1 || res.Failed != 1 {
		t.Fatalf("部分失败结果不符: %+v", res)
	}
	// 非管理员拒绝
	if s, _ := h.do(http.MethodPost, "/api/admin/v1/circles/batch", userTok,
		map[string]any{"action": "ban", "ids": []string{fmt.Sprintf("%d", id)}, "reason": "x"}); s != http.StatusForbidden {
		t.Fatalf("非管理员应403: %d", s)
	}
	_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cir_circle WHERE id=$1`, id)
}

// PRD §5.12.2/§11.4：圈名片数据（圈信息 + 小程序码）+ 重置邀请失效。
func TestIntegrationCircleCardAndInviteReset(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	uid, tok := loginUser(t, h)
	other, otherTok := loginUser(t, h)
	_ = other
	id := time.Now().UnixNano()
	if _, err := h.app.Pool.Exec(ctx, `
		INSERT INTO cir_circle (id, name, intro, owner_user_id, status) VALUES ($1,'名片测试圈','简介',$2,0)`, id, uid); err != nil {
		t.Fatal(err)
	}
	_, env := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/circles/%d/card", id), tok, nil)
	var card struct {
		Name        string `json:"name"`
		Intro       string `json:"intro"`
		MemberCount int    `json:"member_count"`
		OwnerName   string `json:"owner_name"`
		InviteCode  string `json:"invite_code"`
		QR          string `json:"qr_png_base64"`
	}
	_ = json.Unmarshal(env.Data, &card)
	if card.Name != "名片测试圈" || card.InviteCode == "" || card.QR == "" || card.OwnerName == "" {
		t.Fatalf("名片数据不完整: %+v", card)
	}
	// 非成员不可生成
	if s, _ := h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/circles/%d/card", id), otherTok, nil); s != http.StatusForbidden {
		t.Fatalf("非成员生成名片应403: %d", s)
	}
	// 重置邀请：旧码立即失效
	_, env = h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/invite/reset", id), tok, nil)
	var rr struct {
		Invalidated int64 `json:"invalidated"`
	}
	_ = json.Unmarshal(env.Data, &rr)
	if rr.Invalidated < 1 {
		t.Fatalf("重置应失效旧邀请: %+v", env)
	}
	var st int16
	_ = h.app.Pool.QueryRow(ctx, `SELECT status FROM cir_invite WHERE code=$1`, card.InviteCode).Scan(&st)
	if st != 1 {
		t.Fatalf("旧邀请应失效: status=%d", st)
	}
	if s, _ := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/invite/reset", id), otherTok, nil); s != http.StatusForbidden {
		t.Fatalf("非圈主重置应403: %d", s)
	}
	_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cir_invite WHERE circle_id=$1`, id)
	_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cir_circle WHERE id=$1`, id)
}

// PRD §5.12.6/§11.4：内容分享落地页「来自 XX 圈」+ 非成员摘要 + 加入后全文。
func TestIntegrationCircleShareLanding(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	owner, ownerTok := loginUser(t, h)
	_, otherTok := loginUser(t, h)
	_, visitorTok := loginUser(t, h)

	// 实名后发布项目并上架
	_, _ = h.do(http.MethodPost, "/api/mp/v1/certs", ownerTok, map[string]any{
		"cert_type": 1, "real_name": "张三", "id_card_no": "110101199003070019", "agreed": true,
	})
	_, env := h.do(http.MethodPost, "/api/mp/v1/contents", ownerTok, map[string]any{
		"kind": "project", "title": "圈内项目",
		"tagset": map[string]any{"region": "310000", "industry": []string{"A01"}},
		"fields": map[string]any{"amount_wan": 100, "intro": "圈内简介", "core_strength": "优势"},
	})
	var item struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &item)
	if item.ID == 0 {
		t.Fatalf("创建项目失败: %+v", env)
	}
	if _, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/contents/%d/submit", item.ID), ownerTok, nil); e.Code != 0 {
		t.Fatalf("项目上架失败: %+v", e)
	}

	id := time.Now().UnixNano()
	if _, err := h.app.Pool.Exec(ctx, `
		INSERT INTO cir_circle (id, name, owner_user_id, status) VALUES ($1,'分享测试圈',$2,0)`, id, owner); err != nil {
		t.Fatal(err)
	}
	ref := fmt.Sprintf("content.project:%d", item.ID)
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/mounts", id), ownerTok,
		map[string]any{"item_ref": ref}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("挂载失败: %d %+v", s, e)
	}

	path := fmt.Sprintf("/api/mp/v1/aggregate/detail?ref=%s&from_circle=%d", ref, id)
	check := func(tok string, wantPreview bool) map[string]any {
		_, env := h.do(http.MethodGet, path, tok, nil)
		var d struct {
			PreviewOnly bool           `json:"preview_only"`
			FromCircle  map[string]any `json:"from_circle"`
			Content     map[string]any `json:"content"`
		}
		_ = json.Unmarshal(env.Data, &d)
		if d.FromCircle == nil || d.FromCircle["name"] != "分享测试圈" {
			t.Fatalf("缺少来自圈信息: %+v", d)
		}
		if d.PreviewOnly != wantPreview {
			t.Fatalf("preview_only=%v want=%v", d.PreviewOnly, wantPreview)
		}
		if wantPreview {
			if f, _ := d.Content["fields"].(map[string]any); len(f) != 0 {
				t.Fatalf("非成员不应返回字段: %+v", f)
			}
		}
		return d.FromCircle
	}
	check(otherTok, true)

	// 访客实名后加入 → 全文
	_, _ = h.do(http.MethodPost, "/api/mp/v1/certs", visitorTok, map[string]any{
		"cert_type": 1, "real_name": "李四", "id_card_no": "110101199003070027", "agreed": true,
	})
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/mp/v1/circles/%d/apply", id), visitorTok, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("加入圈子失败: %d %+v", s, e)
	}
	fc := check(visitorTok, false)
	if fc["joined"] != true {
		t.Fatalf("成员应为 joined=true: %+v", fc)
	}

	// 已结束：不可加入
	_, _ = h.app.Pool.Exec(ctx, `UPDATE cir_circle SET status=1, end_at=now()-interval '1 hour' WHERE id=$1`, id)
	fc = check(otherTok, true)
	if fc["can_join"] != false {
		t.Fatalf("已结束圈 can_join 应为 false: %+v", fc)
	}

	// 伪造来源：未挂载内容不带 from_circle
	_, env = h.do(http.MethodPost, "/api/mp/v1/contents", ownerTok, map[string]any{
		"kind": "project", "title": "未挂载项目",
		"tagset": map[string]any{"region": "310000", "industry": []string{"A01"}},
		"fields": map[string]any{"amount_wan": 10, "intro": "x", "core_strength": "y"},
	})
	var item2 struct {
		ID int64 `json:"id,string"`
	}
	_ = json.Unmarshal(env.Data, &item2)
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/mp/v1/aggregate/detail?ref=content.project:%d&from_circle=%d", item2.ID, id), otherTok, nil)
	var spoof struct {
		FromCircle any `json:"from_circle"`
	}
	_ = json.Unmarshal(env.Data, &spoof)
	if spoof.FromCircle != nil {
		t.Fatalf("未挂载内容不应带 from_circle: %+v", spoof)
	}
	_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cir_mount WHERE circle_id=$1`, id)
	_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cir_circle WHERE id=$1`, id)
}

// 系统 → 小程序设置：schema 自动出现，超管可热改（AppID/Secret/env_version）。
func TestIntegrationMiniappSettings(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	_, admTok := loginAdmin(t, h)
	defer func() {
		_, _ = h.app.Pool.Exec(ctx, `DELETE FROM sys_settings WHERE key LIKE 'miniapp.%'`)
	}()

	_, env := h.do(http.MethodGet, "/api/admin/v1/settings", admTok, nil)
	var body struct {
		Items []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &body)
	found := false
	for _, it := range body.Items {
		if it.Key == "miniapp.appid" {
			found = true
		}
	}
	if !found {
		t.Fatal("小程序设置未出现在 SettingsSchema")
	}
	if s, e := h.do(http.MethodPut, "/api/admin/v1/settings/miniapp.appid", admTok, map[string]any{"value": "wx-test-appid"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("超管修改小程序 AppID 失败: %d %+v", s, e)
	}
	var v string
	_ = h.app.Pool.QueryRow(ctx, `SELECT value FROM sys_settings WHERE key='miniapp.appid'`).Scan(&v)
	if v != "wx-test-appid" {
		t.Fatalf("小程序 AppID 未热生效: %q", v)
	}
}
