package rbac

import "testing"

// 9.1 五角色 × 权限点默认矩阵（表驱动 + 负向断言）。
func TestDefaultPermissionMatrix(t *testing.T) {
	want := map[string]map[string]bool{
		RoleOps: setOf(
			PermUserRead, PermUserRestrict, PermModeration, PermCertReview, PermBadgeSet, PermBroadcast,
			PermOrderRead, PermCommissionSet, PermOperatorFeeSet, PermExportCreate, PermAuditRead, PermSettingsRead,
			PermSettingsSet, PermJobRun, PermDashboardRead, PermRBACManage, PermOrgManage,
		),
		RoleAuditor: setOf(PermUserRead, PermModeration, PermCertReview, PermDashboardRead),
		RoleFinance: setOf(
			PermUserRead, PermMemberAdjust, PermOrderRead, PermRefundApprove, PermCommissionSet, PermOperatorFeeSet,
			PermSettleReview, PermSettlePay, PermExportCreate, PermAuditRead, PermDashboardRead,
		),
		RoleChannelManager: setOf(
			PermUserRead, PermOperatorReview, PermSettleCreate, PermExportCreate, PermDashboardRead,
		),
	}
	for role, exp := range want {
		got := setOf(defaultPerms[role]...)
		if len(got) != len(exp) {
			t.Fatalf("角色 %s 权限数量不符：got %d want %d", role, len(got), len(exp))
		}
		for p := range exp {
			if !got[p] {
				t.Fatalf("角色 %s 缺权限 %s", role, p)
			}
		}
	}
	// 超管不在默认矩阵（隐含全部）
	if _, ok := defaultPerms[RoleSuper]; ok {
		t.Fatal("超管不应落默认权限矩阵")
	}
	// 每个登记权限点至少授予一个非超管角色（防悬空）
	for _, d := range Registry() {
		covered := false
		for _, perms := range defaultPerms {
			for _, p := range perms {
				if p == d.Perm {
					covered = true
				}
			}
		}
		if !covered {
			t.Fatalf("权限点 %s 未授予任何默认角色", d.Perm)
		}
	}

	// 负向红线：职责分离（运营不碰资金/审核不外溢等）
	deny := map[string][]string{
		RoleOps:            {PermRefundApprove, PermMemberAdjust, PermSettlePay, PermSettleReview},
		RoleAuditor:        {PermUserRestrict, PermRefundApprove, PermSettlePay, PermCommissionSet, PermSettingsSet, PermRBACManage},
		RoleFinance:        {PermUserRestrict, PermModeration, PermCertReview, PermBroadcast, PermRBACManage},
		RoleChannelManager: {PermModeration, PermRefundApprove, PermSettingsSet, PermRBACManage, PermSettlePay},
	}
	for role, perms := range deny {
		got := setOf(defaultPerms[role]...)
		for _, p := range perms {
			if got[p] {
				t.Fatalf("角色 %s 不应拥有 %s（职责分离红线）", role, p)
			}
		}
	}
}

func setOf(ps ...string) map[string]bool {
	m := make(map[string]bool, len(ps))
	for _, p := range ps {
		m[p] = true
	}
	return m
}
