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

// 用户管理：后台新增/编辑用户，列表返回手机号（脱敏）。
func TestIntegrationAdminUserCreateEdit(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	_, admTok := loginAdmin(t, h)
	phone := fmt.Sprintf("139%08d", time.Now().UnixNano()%100000000)
	phone2 := fmt.Sprintf("138%08d", time.Now().UnixNano()%100000000)
	nick := fmt.Sprintf("后台新建-%d", time.Now().UnixNano()%100000)
	defer func() {
		_, _ = h.app.Pool.Exec(ctx, `DELETE FROM idn_user WHERE nickname LIKE '后台新建-%' OR nickname='后台改名'`)
	}()

	_, env := h.do(http.MethodPost, "/api/admin/v1/users", admTok,
		map[string]any{"nickname": nick, "phone": phone, "identity_type": 1})
	var created struct {
		ID          string `json:"id"`
		PhoneMasked string `json:"phone_masked"`
	}
	_ = json.Unmarshal(env.Data, &created)
	if env.Code != 0 || created.ID == "" {
		t.Fatalf("新增用户失败: %+v", env)
	}
	if created.PhoneMasked != phone[:3]+"****"+phone[7:] {
		t.Fatalf("手机号脱敏不符: %q", created.PhoneMasked)
	}

	// 列表显示手机号
	_, env = h.do(http.MethodGet, "/api/admin/v1/governance/users?q="+nick+"&limit=10", admTok, nil)
	var list struct {
		Items []struct {
			ID          string `json:"id"`
			PhoneMasked string `json:"phone_masked"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &list)
	if len(list.Items) == 0 || list.Items[0].PhoneMasked != created.PhoneMasked {
		t.Fatalf("用户列表缺少手机号: %+v", list)
	}

	// 编辑昵称/手机号
	if s, e := h.do(http.MethodPut, "/api/admin/v1/users/"+created.ID, admTok,
		map[string]any{"nickname": "后台改名", "phone": phone2, "identity_type": 2}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("编辑用户失败: %d %+v", s, e)
	}
	var gotNick, gotEnc string
	var gotType int16
	_ = h.app.Pool.QueryRow(ctx, `SELECT nickname, encode(phone_enc,'hex'), identity_type FROM idn_user WHERE id=$1`, created.ID).
		Scan(&gotNick, &gotEnc, &gotType)
	if gotNick != "后台改名" || gotType != 2 || gotEnc == "" {
		t.Fatalf("编辑未生效: %s %d %s", gotNick, gotType, gotEnc)
	}
	// 重复手机号拒绝
	if _, e := h.do(http.MethodPost, "/api/admin/v1/users", admTok,
		map[string]any{"nickname": "重复手机号", "phone": phone2, "identity_type": 1}); e.Code == 0 {
		t.Fatal("重复手机号应失败")
	}
}

// 通知模板可修改：文案/渠道/可关闭即时保存，列表回读一致。
func TestIntegrationNoticeTemplateUpdate(t *testing.T) {
	h := newTestApp(t)
	_, admTok := loginAdmin(t, h)
	code := "circle.DemandPublished.v1"
	_, env := h.do(http.MethodGet, "/api/admin/v1/notice-templates", admTok, nil)
	var list struct {
		Items []struct {
			Code       string   `json:"code"`
			TitleTpl   string   `json:"title_tpl"`
			ContentTpl string   `json:"content_tpl"`
			Channels   []string `json:"channels"`
			Closeable  bool     `json:"closeable"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &list)
	var old struct {
		Title, Content string
		Channels       []string
		Closeable      bool
	}
	for _, it := range list.Items {
		if it.Code == code {
			old.Title, old.Content, old.Channels, old.Closeable = it.TitleTpl, it.ContentTpl, it.Channels, it.Closeable
		}
	}
	if old.Title == "" {
		t.Fatalf("模板 %s 未初始化", code)
	}
	defer func() {
		_, _ = h.do(http.MethodPut, "/api/admin/v1/notice-templates/"+code, admTok, map[string]any{
			"title_tpl": old.Title, "content_tpl": old.Content, "channels": old.Channels, "closeable": old.Closeable,
		})
	}()
	if s, e := h.do(http.MethodPut, "/api/admin/v1/notice-templates/"+code, admTok, map[string]any{
		"title_tpl": "圈内新需求（改）", "content_tpl": "新内容 {title}", "channels": []string{"inapp"}, "closeable": false,
	}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("更新模板失败: %d %+v", s, e)
	}
	_, env = h.do(http.MethodGet, "/api/admin/v1/notice-templates", admTok, nil)
	_ = json.Unmarshal(env.Data, &list)
	found := false
	for _, it := range list.Items {
		if it.Code == code {
			found = it.TitleTpl == "圈内新需求（改）" && it.ContentTpl == "新内容 {title}" && len(it.Channels) == 1 && !it.Closeable
		}
	}
	if !found {
		t.Fatal("模板修改未生效")
	}
}

// 广场后台：圈子详情/统计/内容处置，猫立方详情/成员调整/项目/群聊。
func TestIntegrationAdminPlazaManagement(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	owner, ownerTok := loginUser(t, h)
	member2, _ := loginUser(t, h)
	_, admTok := loginAdmin(t, h)
	cid := time.Now().UnixNano()
	cubeID := cid + 1
	defer func() {
		_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cir_demand WHERE circle_id=$1`, cid)
		_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cir_feed WHERE circle_id=$1`, cid)
		_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cir_mount WHERE circle_id=$1`, cid)
		_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cir_circle WHERE id=$1`, cid)
		_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cub_project WHERE cube_id=$1`, cubeID)
		_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cub_member WHERE cube_id=$1`, cubeID)
		_, _ = h.app.Pool.Exec(ctx, `DELETE FROM cub_cube WHERE id=$1`, cubeID)
	}()
	if _, err := h.app.Pool.Exec(ctx, `
		INSERT INTO cir_circle (id,name,intro,type,owner_user_id,member_count,region_code,status)
		VALUES ($1,'广场测试圈','简介',1,$2,2,'310000',0)`, cid, owner); err != nil {
		t.Fatal(err)
	}
	_, _ = h.app.Pool.Exec(ctx, `INSERT INTO cir_member (id,circle_id,user_ref) VALUES ($1,$2,$3),($4,$2,$5)`,
		cid+10, cid, owner, cid+11, member2)
	if _, err := h.app.Pool.Exec(ctx, `INSERT INTO cir_demand (id,circle_id,author_id,title,content,expire_at,status)
		VALUES ($1,$2,$3,'测试需求','内容',now()+interval '7 day',0)`, cid+20, cid, owner); err != nil {
		t.Fatal(err)
	}
	_, _ = h.app.Pool.Exec(ctx, `INSERT INTO cir_feed (id,circle_id,author_id,text,status) VALUES ($1,$2,$3,'测试动态',0)`, cid+21, cid, owner)
	_, _ = h.app.Pool.Exec(ctx, `INSERT INTO cir_mount (id,circle_id,item_ref,mounted_by,approval_status,status)
		VALUES ($1,$2,'content.project:1',$3,1,0)`, cid+22, cid, owner)

	// 统计 + 详情
	_, env := h.do(http.MethodGet, "/api/admin/v1/circles/stats?days=7", admTok, nil)
	var stats struct {
		NewTotal int `json:"new_total"`
		Members  int `json:"members_total"`
	}
	_ = json.Unmarshal(env.Data, &stats)
	if stats.NewTotal == 0 {
		t.Fatalf("圈子统计未包含新增: %+v", stats)
	}
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/admin/v1/circles/%d", cid), admTok, nil)
	var detail struct {
		Members []struct {
			UserID string `json:"user_id"`
		} `json:"members"`
		Demands []struct {
			ID string `json:"id"`
		} `json:"demands"`
		Feeds []struct {
			ID string `json:"id"`
		} `json:"feeds"`
		Mounts []struct {
			ID string `json:"id"`
		} `json:"mounts"`
	}
	_ = json.Unmarshal(env.Data, &detail)
	if len(detail.Members) < 2 || len(detail.Demands) == 0 || len(detail.Feeds) == 0 || len(detail.Mounts) == 0 {
		t.Fatalf("圈子详情不完整: %+v", detail)
	}
	// 内容处置：删除需求
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/circles/%d/demands/%d/remove", cid, cid+20), admTok,
		map[string]any{"reason": "违规"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("删除需求失败: %d %+v", s, e)
	}
	var st int16
	_ = h.app.Pool.QueryRow(ctx, `SELECT status FROM cir_demand WHERE id=$1`, cid+20).Scan(&st)
	if st != 2 {
		t.Fatalf("需求应软删: %d", st)
	}

	// 猫立方详情/项目/成员调整
	if _, err := h.app.Pool.Exec(ctx, `
		INSERT INTO cub_cube (id,name,leader_id,honey_id,plan_level,status) VALUES ($1,'立方测试',$2,$3,1,0)`,
		cubeID, owner, member2); err != nil {
		t.Fatal(err)
	}
	_, _ = h.app.Pool.Exec(ctx, `INSERT INTO cub_member (id,cube_id,user_ref,role,status) VALUES
		($1,$2,$3,1,0),($4,$2,$5,2,0),($6,$2,$7,3,0)`, cubeID+10, cubeID, owner, cubeID+11, member2, cubeID+12, cubeID+13)
	other, _ := loginUser(t, h)
	_, _ = h.app.Pool.Exec(ctx, `INSERT INTO cub_member (id,cube_id,user_ref,role,status) VALUES ($1,$2,$3,3,0)`, cubeID+14, cubeID, other)
	_, _ = h.app.Pool.Exec(ctx, `INSERT INTO cub_project (id,cube_id,type,name,leader_id,progress,status)
		VALUES ($1,$2,1,'研究项目',$3,40,0)`, cubeID+20, cubeID, owner)
	_, env = h.do(http.MethodGet, fmt.Sprintf("/api/admin/v1/cubes/%d", cubeID), admTok, nil)
	var cd struct {
		Cube struct {
			Name string `json:"name"`
		} `json:"cube"`
		Members []struct {
			UserID string `json:"user_id"`
		} `json:"members"`
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	_ = json.Unmarshal(env.Data, &cd)
	if cd.Cube.Name != "立方测试" || len(cd.Members) < 4 || len(cd.Projects) != 1 {
		t.Fatalf("立方详情不完整: %+v", cd)
	}
	// 调整猫蜜为 other
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/cubes/%d/honey", cubeID), admTok,
		map[string]any{"user_id": fmt.Sprintf("%d", other)}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("调整猫蜜失败: %d %+v", s, e)
	}
	var honey int64
	_ = h.app.Pool.QueryRow(ctx, `SELECT honey_id FROM cub_cube WHERE id=$1`, cubeID).Scan(&honey)
	if honey != other {
		t.Fatalf("猫蜜未更新: %d", honey)
	}
	// 平台移除成员
	if s, e := h.do(http.MethodPost, fmt.Sprintf("/api/admin/v1/cubes/%d/members/%d/remove", cubeID, other), admTok,
		map[string]any{"reason": "违规"}); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("移除成员失败: %d %+v", s, e)
	}
	// 群聊记录（无会话时为空数组）
	if s, e := h.do(http.MethodGet, fmt.Sprintf("/api/admin/v1/cubes/%d/chats", cubeID), admTok, nil); s != http.StatusOK || e.Code != 0 {
		t.Fatalf("群聊记录查询失败: %d %+v", s, e)
	}
	_ = ownerTok
}
