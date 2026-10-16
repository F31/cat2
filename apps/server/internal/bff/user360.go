package bff

import (
	"net/http"
	"strconv"

	contentcontract "catcube/internal/modules/content/contract"
)

// user360 用户 360（p4 §4.1）：只读聚合，每区标注来源模块；不提供写操作。
// 说明：与「渠道/佣金」相关区块依赖 channel 模块（#13），此处不包含。
func (a *Aggregator) handleUser360(w http.ResponseWriter, req *http.Request) {
	uid, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil || uid <= 0 {
		writeErr(w, 1, "参数错误")
		return
	}
	ctx := req.Context()
	out := map[string]any{"user_id": strconv.FormatInt(uid, 10), "sections": map[string]any{}}
	sections := out["sections"].(map[string]any)

	// identity：账号状态与实名
	if brief, err := a.deps.Identity.UserBrief(ctx, uid); err == nil {
		realname := int16(0)
		if ok, err := a.deps.Verify.IsRealname(ctx, uid); err == nil && ok {
			realname = 1
		}
		sections["identity"] = map[string]any{
			"module": "identity", "brief": brief, "realname_status": realname,
		}
	}
	// profile：名片与画像完成度
	if card, err := a.deps.Profile.CardOf(ctx, uid); err == nil {
		if comp, cerr := a.deps.Profile.CompletionOf(ctx, uid); cerr == nil {
			sections["profile"] = map[string]any{"module": "profile", "card": card, "completion": comp}
		} else {
			sections["profile"] = map[string]any{"module": "profile", "card": card}
		}
	}
	// verify：认证记录 + 单位归属 + 组织的管理员
	if certs, err := a.deps.Verify.CertsOf(ctx, uid); err == nil {
		affs, _ := a.deps.Verify.AffiliationsOf(ctx, uid)
		admins, _ := a.deps.Verify.OrgAdminsOf(ctx, uid)
		sections["verify"] = map[string]any{"module": "verify", "certs": certs, "affiliations": affs, "org_admins": admins}
	}
	// membership：会员与子账号
	section := map[string]any{"module": "membership"}
	if m, ok, err := a.deps.Membership.Me(ctx, uid); err == nil && ok {
		section["member"] = m
	}
	if subs, err := a.deps.Membership.SubAccounts(ctx, uid); err == nil && len(subs) > 0 {
		section["sub_accounts"] = subs
	}
	sections["membership"] = section
	// content：发布列表（四类）
	if items, err := a.deps.Content.ListByOwner(ctx, uid, ""); err == nil {
		sections["content"] = map[string]any{"module": "content", "items": items}
	}
	// cube：所属猫立方与角色
	if cubes, err := a.deps.Cube.ListMine(ctx, uid); err == nil {
		roles := make([]map[string]any, 0, len(cubes))
		for _, c := range cubes {
			role, _ := a.deps.Cube.RoleOf(ctx, c.ID, uid)
			roles = append(roles, map[string]any{"cube": c, "role": role})
		}
		sections["cube"] = map[string]any{"module": "cube", "cubes": roles}
	}
	// circle：所属圈子与角色
	if circles, err := a.deps.Circle.ListMine(ctx, uid); err == nil {
		roles := make([]map[string]any, 0, len(circles))
		for _, c := range circles {
			role, _ := a.deps.Circle.RoleOf(ctx, c.ID, uid)
			roles = append(roles, map[string]any{"circle": c, "role": role})
		}
		sections["circle"] = map[string]any{"module": "circle", "circles": roles}
	}
	// trust：违规/限制状态与相关审核任务（只读）
	if restricted, err := a.deps.Trust.IsRestricted(ctx, uid); err == nil {
		sections["trust"] = map[string]any{"module": "trust", "restricted": restricted}
	}
	out["note"] = "只读聚合；写操作请使用对应治理入口（封禁/限制/代操作）。"
	writeData(w, out)
}

// 保持 content 契约引用（ListByOwner 的 kind 空串=全部，语义由 content 模块定义）。
var _ = contentcontract.StatusPublished
