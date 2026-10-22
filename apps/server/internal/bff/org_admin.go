package bff

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	idcontract "catcube/internal/modules/identity/contract"
	verifycontract "catcube/internal/modules/verify/contract"
)

// 后台组织管理（运营）：组织账号 + 组织名片 + 认证 + 成员 + 管理员聚合。
// 组织主体沿用现有模型：idn_user.identity_type=2，org_id 即组织账号 ID。

type orgCategory struct {
	Code  string
	Label string
}

var orgCategories = []orgCategory{
	{Code: "cooperation_circle", Label: "合作圈"},
	{Code: "association", Label: "商协会 / 校友会"},
	{Code: "research", Label: "大学 / 科研"},
	{Code: "service_org", Label: "服务机构"},
	{Code: "company", Label: "企业"},
	{Code: "business_person", Label: "商务人士"},
}

func orgCategoryLabel(code string) string {
	for _, c := range orgCategories {
		if c.Code == code {
			return c.Label
		}
	}
	if code == "gov_service" {
		return "服务机构（政府）"
	}
	return "未分类"
}

func (a *Aggregator) handleAdminOrgs(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	q := strings.TrimSpace(req.URL.Query().Get("q"))
	category := strings.TrimSpace(req.URL.Query().Get("category"))
	status := int16(-1)
	if s := req.URL.Query().Get("status"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			status = int16(v)
		}
	}
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := a.deps.Identity.AdminSearchUsers(ctx, idcontract.AdminUserFilter{
		Status: status, RealnameStatus: -1, IdentityType: 2, Limit: 200,
	})
	if err != nil {
		writeErrCode(w, http.StatusInternalServerError, 9, "服务错误")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	needle := strings.ToLower(q)
	for _, r := range rows {
		card, _ := a.deps.Profile.CardOf(ctx, r.ID)
		if needle != "" {
			hay := strings.ToLower(strings.Join([]string{card.Name, r.Nickname, r.RealName}, " "))
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		if category != "" && card.OrgTypeCode != category {
			continue
		}
		items = append(items, a.orgListItem(ctx, r.ID, r.Nickname, r.Status, r.RealnameStatus, r.PhoneMasked,
			r.RealName, r.IDCardMasked, r.RegionCode, r.CreatedAt,
			card.Name, card.RegionCode, card.IndustryCodes, card.OrgTypeCode))
		if len(items) >= limit {
			break
		}
	}
	writeData(w, map[string]any{"items": items, "total": len(items)})
}

func (a *Aggregator) orgListItem(ctx context.Context, id int64, nickname string, status, realnameStatus int16, phoneMasked,
	realName, idCardMasked, identityRegion string, createdAt int64, cardName, cardRegion string,
	industry []string, orgType string) map[string]any {
	name := cardName
	if strings.TrimSpace(name) == "" {
		name = nickname
	}
	region := cardRegion
	if region == "" {
		region = identityRegion
	}
	item := map[string]any{
		"id": strconv.FormatInt(id, 10), "name": name, "nickname": nickname,
		"status": status, "realname_status": realnameStatus, "phone_masked": phoneMasked, "real_name": realName,
		"id_card_masked": idCardMasked, "category": orgType, "category_label": orgCategoryLabel(orgType),
		"region_code": region, "industry_codes": industry, "created_at": createdAt,
	}
	if certs, err := a.deps.Verify.CertsOf(ctx, id); err == nil {
		if cert := latestOrgCert(certs); cert != nil {
			item["cert_status"] = cert.Status
			item["cert_type"] = cert.CertType
			item["cert_submitted_at"] = cert.SubmittedAt
		}
	}
	if affs, err := a.deps.Verify.AffiliationsOfOrg(ctx, id); err == nil {
		item["member_count"] = countAff(affs, 1)
		item["member_pending"] = countAff(affs, 0)
	}
	if admins, err := a.deps.Verify.OrgAdminsOf(ctx, id); err == nil {
		n := 0
		for _, ad := range admins {
			if ad.Status == 1 {
				n++
			}
		}
		item["admin_count"] = n
	}
	return item
}

func latestOrgCert(certs []verifycontract.Cert) *verifycontract.Cert {
	var out *verifycontract.Cert
	for i := range certs {
		c := &certs[i]
		if c.CertType != verifycontract.CertEnterprise && c.CertType != verifycontract.CertInstitution {
			continue
		}
		if out == nil || c.SubmittedAt.After(out.SubmittedAt) {
			out = c
		}
	}
	return out
}

func countAff(affs []verifycontract.Affiliation, status int16) int {
	n := 0
	for _, a := range affs {
		if a.Status == status {
			n++
		}
	}
	return n
}

func (a *Aggregator) handleAdminOrgDetail(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeErrCode(w, http.StatusBadRequest, 1, "参数错误")
		return
	}
	out := map[string]any{"id": strconv.FormatInt(id, 10)}
	if brief, err := a.deps.Identity.UserBrief(ctx, id); err == nil {
		out["user"] = brief
	}
	if card, err := a.deps.Profile.CardOf(ctx, id); err == nil {
		out["card"] = card
	}
	if certs, err := a.deps.Verify.CertsOf(ctx, id); err == nil {
		out["certs"] = certs
	}
	if affs, err := a.deps.Verify.AffiliationsOfOrg(ctx, id); err == nil {
		out["members"] = affs
	}
	if admins, err := a.deps.Verify.OrgAdminsOf(ctx, id); err == nil {
		out["admins"] = admins
	}
	writeData(w, out)
}
