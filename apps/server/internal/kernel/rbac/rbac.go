// Package rbac 后台 RBAC（p4 §3，ADR-002）：
// 五角色 + 权限点矩阵为**默认种子**，除超管外角色权限可配置（支持自定义角色）。
// 超管固定：以 `idn_user.admin_role>0` 为准（运维引导 `catcube grant-admin`），隐含全部权限、不可编辑/删除。
// 服务端为唯一权威；反提权（只能授予自身权限子集）与防锁死（保证恒有超管）在此强制。
package rbac

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"catcube/internal/kernel/db"
)

// 角色编码（内置五角色）。
const (
	RoleSuper          = "super"
	RoleOps            = "ops"
	RoleAuditor        = "auditor"
	RoleFinance        = "finance"
	RoleChannelManager = "channel_manager"
)

// 权限点（<module>:<resource>:<action>，只增不改名）。
const (
	PermUserRead       = "governance:user:read"
	PermUserRestrict   = "governance:user:restrict"
	PermModeration     = "content:moderation:decide"
	PermCertReview     = "verify:cert:review"
	PermBadgeSet       = "verify:badge:set"
	PermBroadcast      = "notice:announcement:broadcast"
	PermMemberAdjust   = "membership:member:adjust"
	PermRefundApprove  = "trade:refund:approve"
	PermCommissionSet  = "channel:commission_rule:set"
	PermOperatorFeeSet = "channel:operator_fee:set"
	PermOperatorReview = "channel:operator:review"
	PermSettleCreate   = "channel:settlement:create"
	PermSettleReview   = "channel:settlement:review"
	PermSettlePay      = "channel:settlement:pay"
	PermExportCreate   = "ops:export:create"
	PermAuditRead      = "ops:audit:read"
	PermSettingsSet    = "ops:settings:set"
	PermJobRun         = "ops:job:run"
	PermDashboardRead  = "ops:dashboard:read"
	PermRBACManage     = "ops:rbac:manage"
)

// Role 角色。
type Role struct {
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	Protected bool     `json:"protected"` // 超管：不可编辑/删除
	Builtin   bool     `json:"builtin"`   // 内置角色：不可删除
	Perms     []string `json:"perms,omitempty"`
}

// PermDef 权限点定义（后台「角色与权限」页展示）。
type PermDef struct {
	Perm  string `json:"perm"`
	Label string `json:"label"`
}

// buildinRoles 内置角色（超管 protected；其余默认权限见 defaultPerms）。
var builtinRoles = []Role{
	{Code: RoleSuper, Name: "超级管理员", Protected: true, Builtin: true},
	{Code: RoleOps, Name: "运营", Builtin: true},
	{Code: RoleAuditor, Name: "审核员", Builtin: true},
	{Code: RoleFinance, Name: "财务", Builtin: true},
	{Code: RoleChannelManager, Name: "渠道经理", Builtin: true},
}

// defaultPerms 默认权限矩阵（decision-checklist-13 §2.2 种子；超管隐含全部，不列）。
var defaultPerms = map[string][]string{
	RoleOps: {
		PermUserRead, PermUserRestrict, PermModeration, PermCertReview, PermBadgeSet, PermBroadcast,
		PermCommissionSet, PermOperatorFeeSet, PermExportCreate, PermAuditRead, PermSettingsSet, PermJobRun,
		PermDashboardRead, PermRBACManage,
	},
	RoleAuditor: {PermUserRead, PermModeration, PermCertReview, PermDashboardRead},
	RoleFinance: {
		PermUserRead, PermMemberAdjust, PermRefundApprove, PermCommissionSet, PermOperatorFeeSet,
		PermSettleReview, PermSettlePay, PermExportCreate, PermAuditRead, PermDashboardRead,
	},
	RoleChannelManager: {
		PermUserRead, PermOperatorReview, PermSettleCreate, PermExportCreate, PermDashboardRead,
	},
}

// permLabels 权限点中文名（后台展示）。
var permLabels = map[string]string{
	PermUserRead: "用户查看/检索", PermUserRestrict: "用户封禁/限制",
	PermModeration: "内容审核处置", PermCertReview: "认证审核", PermBadgeSet: "标识配置/授予",
	PermBroadcast: "定向公告", PermMemberAdjust: "会员人工调整", PermRefundApprove: "订单退款",
	PermCommissionSet: "佣金规则配置", PermOperatorFeeSet: "代理费/保证金口径配置",
	PermOperatorReview: "运营商开户审核", PermSettleCreate: "结算发起", PermSettleReview: "结算复核",
	PermSettlePay: "结算打款", PermExportCreate: "导出中心", PermAuditRead: "审计查询",
	PermSettingsSet: "系统配置/特性开关", PermJobRun: "任务重跑/死信重放",
	PermDashboardRead: "看板/埋点", PermRBACManage: "角色与权限管理",
}

// Registry 全部权限点（按定义顺序，稳定）。
func Registry() []PermDef {
	order := []string{
		PermUserRead, PermUserRestrict, PermModeration, PermCertReview, PermBadgeSet, PermBroadcast,
		PermMemberAdjust, PermRefundApprove, PermCommissionSet, PermOperatorFeeSet, PermOperatorReview,
		PermSettleCreate, PermSettleReview, PermSettlePay, PermExportCreate, PermAuditRead,
		PermSettingsSet, PermJobRun, PermDashboardRead, PermRBACManage,
	}
	out := make([]PermDef, 0, len(order))
	for _, p := range order {
		out = append(out, PermDef{Perm: p, Label: permLabels[p]})
	}
	return out
}

// Schema 内核表迁移（p4 §3，ADR-002）。
const Schema = `
CREATE TABLE IF NOT EXISTS sys_admin_user (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id bigint NOT NULL UNIQUE,     -- 关联平台用户（idn_user.id）
	account varchar(64),
	status smallint NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sys_admin_role (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	code varchar(32) NOT NULL UNIQUE,
	name varchar(64) NOT NULL,
	protected boolean NOT NULL DEFAULT false,  -- 超管：不可编辑/删除
	builtin boolean NOT NULL DEFAULT false,    -- 内置：不可删除
	space_id smallint,                          -- 数据范围（门店/区域维度）
	region_code varchar(16),
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sys_admin_user_role (
	user_id bigint NOT NULL,
	role_id bigint NOT NULL,
	PRIMARY KEY (user_id, role_id)
);

CREATE TABLE IF NOT EXISTS sys_admin_role_permission (
	role_id bigint NOT NULL,
	perm varchar(64) NOT NULL,
	PRIMARY KEY (role_id, perm)
);
`

// Store RBAC 存储与解析。
type Store struct{ pool *db.Pool }

// New 构造。
func New(pool *db.Pool) *Store { return &Store{pool: pool} }

// Seed 幂等写入内置角色与默认权限：角色首次创建时写默认权限（此后由运营配置，不回填）。
func (s *Store) Seed(ctx context.Context) error {
	for _, r := range builtinRoles {
		tag, err := s.pool.Exec(ctx, `
			INSERT INTO sys_admin_role (code, name, protected, builtin) VALUES ($1,$2,$3,$4)
			ON CONFLICT (code) DO NOTHING`, r.Code, r.Name, r.Protected, r.Builtin)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 || r.Protected {
			continue // 已存在（不覆盖运营修改）；超管不存权限（隐含全部）
		}
		for _, p := range defaultPerms[r.Code] {
			if _, err := s.pool.Exec(ctx, `
				INSERT INTO sys_admin_role_permission (role_id, perm)
				SELECT id, $2 FROM sys_admin_role WHERE code=$1
				ON CONFLICT DO NOTHING`, r.Code, p); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsSuper 是否超管（引导态 `idn_user.admin_role>0`）。
func (s *Store) IsSuper(ctx context.Context, userID int64) (bool, error) {
	var role int16
	if err := s.pool.QueryRow(ctx, `SELECT admin_role FROM idn_user WHERE id=$1`, userID).Scan(&role); err != nil {
		return false, nil
	}
	return role > 0, nil
}

// IsAdmin 后台准入：超管或拥有任一角色。
func (s *Store) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	if super, _ := s.IsSuper(ctx, userID); super {
		return true, nil
	}
	var n int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(1) FROM sys_admin_user_role ur
		JOIN sys_admin_user au ON au.id=ur.user_id AND au.status=1
		WHERE au.user_id=$1`, userID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// Perms 用户有效权限（超管=全部；否则角色权限并集）。
func (s *Store) Perms(ctx context.Context, userID int64) ([]string, error) {
	if super, _ := s.IsSuper(ctx, userID); super {
		all := make([]string, 0, len(permLabels))
		for _, d := range Registry() {
			all = append(all, d.Perm)
		}
		return all, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT rp.perm FROM sys_admin_user_role ur
		JOIN sys_admin_user au ON au.id=ur.user_id AND au.status=1
		JOIN sys_admin_role_permission rp ON rp.role_id=ur.role_id
		WHERE au.user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out, rows.Err()
}

// HasPerm 是否具备某权限。
func (s *Store) HasPerm(ctx context.Context, userID int64, perm string) (bool, error) {
	perms, err := s.Perms(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, p := range perms {
		if p == perm {
			return true, nil
		}
	}
	return false, nil
}

// Roles 全部角色（含各类权限）。
func (s *Store) Roles(ctx context.Context) ([]Role, error) {
	rows, err := s.pool.Query(ctx, `SELECT code, name, protected, builtin FROM sys_admin_role ORDER BY protected DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.Code, &r.Name, &r.Protected, &r.Builtin); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Protected {
			continue // 超管隐含全部
		}
		ps, err := s.RolePerms(ctx, out[i].Code)
		if err != nil {
			return nil, err
		}
		out[i].Perms = ps
	}
	return out, nil
}

// RolePerms 角色权限列表。
func (s *Store) RolePerms(ctx context.Context, code string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT rp.perm FROM sys_admin_role_permission rp
		JOIN sys_admin_role r ON r.id=rp.role_id WHERE r.code=$1 ORDER BY rp.perm`, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// roleByCode 取角色（不存在返回 nil, nil）。
func (s *Store) roleByCode(ctx context.Context, code string) (*Role, error) {
	var r Role
	err := s.pool.QueryRow(ctx,
		`SELECT code, name, protected, builtin FROM sys_admin_role WHERE code=$1`, code).
		Scan(&r.Code, &r.Name, &r.Protected, &r.Builtin)
	if err != nil {
		return nil, nil
	}
	return &r, nil
}

// SetRolePerms 设置角色权限（超管不可改；权限点须在注册表内）。
// editorPerms 为操作者权限：非超管只能授予自身权限子集（反提权）。
func (s *Store) SetRolePerms(ctx context.Context, code string, perms []string, editorIsSuper bool, editorPerms []string) error {
	r, err := s.roleByCode(ctx, code)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("角色不存在")
	}
	if r.Protected {
		return fmt.Errorf("超管角色不可修改")
	}
	set := map[string]bool{}
	for _, p := range Registry() {
		set[p.Perm] = true
	}
	own := map[string]bool{}
	for _, p := range editorPerms {
		own[p] = true
	}
	for _, p := range perms {
		if !set[p] {
			return fmt.Errorf("未知权限点: %s", p)
		}
		if !editorIsSuper && !own[p] {
			return fmt.Errorf("反提权：无权授予 %s", p)
		}
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM sys_admin_role_permission WHERE role_id=(SELECT id FROM sys_admin_role WHERE code=$1)`, code); err != nil {
			return err
		}
		for _, p := range perms {
			if _, err := tx.Exec(ctx,
				`INSERT INTO sys_admin_role_permission (role_id, perm) SELECT id, $2 FROM sys_admin_role WHERE code=$1 ON CONFLICT DO NOTHING`,
				code, p); err != nil {
				return err
			}
		}
		return nil
	})
}

// CreateRole 创建自定义角色（非内置）。
func (s *Store) CreateRole(ctx context.Context, code, name string) error {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if code == "" || name == "" {
		return fmt.Errorf("角色编码/名称不能为空")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sys_admin_role (code, name, protected, builtin) VALUES ($1,$2,false,false)
		ON CONFLICT (code) DO NOTHING`, code, name)
	return err
}

// DeleteRole 删除角色（超管/内置不可删；有在用账号不可删）。
func (s *Store) DeleteRole(ctx context.Context, code string) error {
	r, err := s.roleByCode(ctx, code)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("角色不存在")
	}
	if r.Protected || r.Builtin {
		return fmt.Errorf("内置/超管角色不可删除")
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(1) FROM sys_admin_user_role WHERE role_id=(SELECT id FROM sys_admin_role WHERE code=$1)`, code).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("角色仍被 %d 个账号使用", n)
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM sys_admin_role_permission WHERE role_id=(SELECT id FROM sys_admin_role WHERE code=$1)`, code); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM sys_admin_role WHERE code=$1`, code)
		return err
	})
}

// AssignRoles 设置账号角色（替换式）。反提权：非超管只能分配权限不超出自身的角色。
// 防锁死：不得使系统失去全部 rbac:manage 持有者（超管身份不因分配丢失）。
func (s *Store) AssignRoles(ctx context.Context, userID int64, codes []string, editorIsSuper bool, editorPerms []string) error {
	own := map[string]bool{}
	for _, p := range editorPerms {
		own[p] = true
	}
	for _, c := range codes {
		r, err := s.roleByCode(ctx, c)
		if err != nil {
			return err
		}
		if r == nil {
			return fmt.Errorf("角色不存在: %s", c)
		}
		if !editorIsSuper && !r.Protected {
			perms, err := s.RolePerms(ctx, c)
			if err != nil {
				return err
			}
			for _, p := range perms {
				if !own[p] {
					return fmt.Errorf("反提权：无权分配含 %s 的角色", p)
				}
			}
		}
	}
	// 目标用户是否超管（引导态）：超管身份不因角色分配而失去
	super, _ := s.IsSuper(ctx, userID)
	if !super {
		// 变更后是否仍有 rbac:manage 持有者
		keep := false
		for _, c := range codes {
			perms, err := s.RolePerms(ctx, c)
			if err != nil {
				return err
			}
			for _, p := range perms {
				if p == PermRBACManage {
					keep = true
				}
			}
		}
		if !keep {
			n, err := s.countRBACHolders(ctx, userID)
			if err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("防锁死：必须保留至少一个具备 %s 的账号", PermRBACManage)
			}
		}
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		var adminID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO sys_admin_user (user_id, status) VALUES ($1,1)
			ON CONFLICT (user_id) DO UPDATE SET status=1 RETURNING id`, userID).Scan(&adminID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM sys_admin_user_role WHERE user_id=$1`, adminID); err != nil {
			return err
		}
		for _, c := range codes {
			if _, err := tx.Exec(ctx, `
				INSERT INTO sys_admin_user_role (user_id, role_id)
				SELECT $1, id FROM sys_admin_role WHERE code=$2 ON CONFLICT DO NOTHING`, adminID, c); err != nil {
				return err
			}
		}
		return nil
	})
}

// AdminRoles 某账号当前角色编码。
func (s *Store) AdminRoles(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.code FROM sys_admin_user_role ur
		JOIN sys_admin_user au ON au.id=ur.user_id
		JOIN sys_admin_role r ON r.id=ur.role_id
		WHERE au.user_id=$1 ORDER BY r.code`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// countSuperAdmins 超管数量。
func (s *Store) countSuperAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(1) FROM idn_user WHERE admin_role>0`).Scan(&n)
	return n, err
}

// countRBACHolders 除 excluding 外，仍持有 rbac:manage 的账号数（超管计入）。
func (s *Store) countRBACHolders(ctx context.Context, excluding int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(DISTINCT au.user_id) FROM sys_admin_user_role ur
		JOIN sys_admin_user au ON au.id=ur.user_id AND au.status=1
		JOIN sys_admin_role_permission rp ON rp.role_id=ur.role_id
		WHERE rp.perm=$1 AND au.user_id<>$2`, PermRBACManage, excluding).Scan(&n)
	if err != nil {
		return 0, err
	}
	supers, err := s.countSuperAdmins(ctx)
	if err != nil {
		return 0, err
	}
	return n + supers, nil
}

// VisibleModules 可见模块集合（超管=全部；否则按权限名前缀的模块）。
func VisibleModules(perms []string) map[string]bool {
	mods := map[string]bool{}
	for _, p := range perms {
		if i := strings.Index(p, ":"); i > 0 {
			mods[p[:i]] = true
		}
	}
	return mods
}
