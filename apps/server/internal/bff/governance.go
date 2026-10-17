package bff

import (
	"net/http"
	"strconv"

	idcontract "catcube/internal/modules/identity/contract"
)

// handleGovernanceUsers 后台用户检索（p4 §4.2）：identity 组合筛选 + 会员等级过滤（membership 交集）。
// 手机号走盲索引精确匹配；返回仅掩码字段；导出请走导出中心（带审计）。
func (a *Aggregator) handleGovernanceUsers(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	f := idcontract.AdminUserFilter{
		Status:         int16(atoiDefault(q.Get("status"), -1)),
		Q:              q.Get("q"),
		Phone:          q.Get("phone"),
		RealnameStatus: int16(atoiDefault(q.Get("realname"), -1)),
		IdentityType:   int16(atoiDefault(q.Get("identity_type"), -1)),
		FromUnix:       int64(atoiDefault(q.Get("from"), 0)),
		ToUnix:         int64(atoiDefault(q.Get("to"), 0)),
		Limit:          atoiDefault(q.Get("limit"), 50),
		Offset:         atoiDefault(q.Get("offset"), 0),
	}
	rows, err := a.deps.Identity.AdminSearchUsers(req.Context(), f)
	if err != nil {
		writeErrCode(w, http.StatusInternalServerError, 9, "服务错误")
		return
	}
	// 会员等级过滤：取该等级生效会员集合做交集（level>0 时）
	level := atoiDefault(q.Get("level"), 0)
	if level > 0 {
		ids, lerr := a.deps.Membership.UserIDsByLevel(req.Context(), int16(level), 500)
		if lerr != nil {
			writeErrCode(w, http.StatusInternalServerError, 9, "服务错误")
			return
		}
		set := make(map[int64]bool, len(ids))
		for _, id := range ids {
			set[id] = true
		}
		filtered := rows[:0]
		for _, r := range rows {
			if set[r.ID] {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	// 标注会员等级（批量，便于后台展示）
	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		lvl, active, _ := a.deps.Membership.LevelOf(req.Context(), r.ID)
		items = append(items, map[string]any{
			"id": strconv.FormatInt(r.ID, 10), "nickname": r.Nickname, "status": r.Status,
			"identity_type": r.IdentityType, "realname_status": r.RealnameStatus,
			"phone_masked": r.PhoneMasked, "created_at": r.CreatedAt,
			"member_level": lvl, "member_active": active,
		})
	}
	writeData(w, map[string]any{"items": items, "limit": f.Limit, "offset": f.Offset})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}
