// Package menu 后台菜单：DB 为唯一数据源，启动时幂等初始化默认菜单，接口按 RBAC 动态过滤。
package menu

import (
	"context"

	"catcube/internal/kernel/db"
)

// Schema 菜单表（sys_ 前缀）。
const Schema = `
CREATE TABLE IF NOT EXISTS sys_admin_menu (
	key varchar(64) PRIMARY KEY,
	label varchar(64) NOT NULL,
	path varchar(128) NOT NULL UNIQUE,
	module varchar(32) NOT NULL,
	group_key varchar(32) NOT NULL,
	group_label varchar(64) NOT NULL,
	group_order int NOT NULL DEFAULT 99,
	item_order int NOT NULL DEFAULT 99,
	required_perm varchar(64) NOT NULL DEFAULT '',
	super_only boolean NOT NULL DEFAULT false,
	enabled boolean NOT NULL DEFAULT true,
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sys_admin_menu_order ON sys_admin_menu(group_order, item_order);
`

// Item 菜单项（JSON 字段与前端约定一致）。
type Item struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Path         string `json:"path"`
	Module       string `json:"module"`
	Group        string `json:"group"`
	GroupLabel   string `json:"group_label"`
	GroupOrder   int    `json:"group_order"`
	Order        int    `json:"order"`
	RequiredPerm string `json:"required_perm,omitempty"`
	SuperOnly    bool   `json:"super_only,omitempty"`
	Enabled      bool   `json:"enabled"`
}

// Store 菜单存储。
type Store struct{ pool *db.Pool }

func New(pool *db.Pool) *Store { return &Store{pool: pool} }

// Seed 幂等写入默认菜单：已存在（可能被运营改过）不覆盖。
func (s *Store) Seed(ctx context.Context, items []Item) error {
	for _, it := range items {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO sys_admin_menu
				(key,label,path,module,group_key,group_label,group_order,item_order,required_perm,super_only,enabled)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (key) DO NOTHING`,
			it.Key, it.Label, it.Path, it.Module, it.Group, it.GroupLabel, it.GroupOrder, it.Order,
			it.RequiredPerm, it.SuperOnly, it.Enabled); err != nil {
			return err
		}
	}
	return nil
}

// List 全部启用菜单（按一级/组内顺序）。
func (s *Store) List(ctx context.Context) ([]Item, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT key,label,path,module,group_key,group_label,group_order,item_order,required_perm,super_only,enabled
		FROM sys_admin_menu WHERE enabled=true
		ORDER BY group_order, item_order, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.Key, &it.Label, &it.Path, &it.Module, &it.Group, &it.GroupLabel,
			&it.GroupOrder, &it.Order, &it.RequiredPerm, &it.SuperOnly, &it.Enabled); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
