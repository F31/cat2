// Package featureflag 特性开关（灰度）：按用户/比例/白名单放量；后台可配。
// 用于灰度发布（p6）：新功能先在白名单/比例内放量，出问题关开关即回退。
package featureflag

import (
	"context"
	"encoding/json"
	"hash/fnv"

	"catcube/internal/kernel/db"
)

// Flag 开关定义。Enabled=false 时一律关闭；Rules 为空时 Enabled=true 即全量。
type Flag struct {
	Key     string
	Enabled bool
	Rules   Rules
}

// Rules 灰度规则。
type Rules struct {
	Users []int64 `json:"users,omitempty"` // 白名单（命中即开）
	Ratio int     `json:"ratio,omitempty"` // 比例放量 0-100（稳定哈希，按用户粘性）
}

// KeyDef 可配置的开关键（后台下拉；与各模块常量保持一致）。
type KeyDef struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Known 已知开关键（新增功能时在此登记；AI 场景来自 ai/contract）。
var Known = []KeyDef{
	{Key: "ai.onboard", Label: "AI 一句话入驻", Description: "生成入驻草稿"},
	{Key: "ai.publish", Label: "AI 发布草稿", Description: "内容发布草稿生成"},
	{Key: "ai.search", Label: "AI 自然语言搜索", Description: "搜索条件解析"},
	{Key: "ai.review", Label: "AI 审核辅助", Description: "审核预分类/摘要"},
	{Key: "ai.cs", Label: "AI 智能客服", Description: "平台规则问答"},
}

// Store 开关存储（sys_feature_flag）。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// Get 读取开关。
func (s *Store) Get(ctx context.Context, key string) (Flag, bool, error) {
	var enabled bool
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT enabled, rules FROM sys_feature_flag WHERE key=$1`, key).Scan(&enabled, &raw)
	if err != nil {
		return Flag{}, false, nil // 未登记视为不存在（默认关闭）
	}
	f := Flag{Key: key, Enabled: enabled}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &f.Rules)
	}
	return f, true, nil
}

// Set 写入开关（后台配置）。
func (s *Store) Set(ctx context.Context, f Flag) error {
	raw, _ := json.Marshal(f.Rules)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sys_feature_flag (key, enabled, rules, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (key) DO UPDATE SET enabled=EXCLUDED.enabled, rules=EXCLUDED.rules, updated_at=now()`,
		f.Key, f.Enabled, raw)
	return err
}

// List 全部开关。
func (s *Store) List(ctx context.Context) ([]Flag, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, enabled, rules FROM sys_feature_flag ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Flag
	for rows.Next() {
		var f Flag
		var raw []byte
		if err := rows.Scan(&f.Key, &f.Enabled, &raw); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &f.Rules)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Eval 评估某用户是否命中开关（未登记=false）。
func Eval(f Flag, userID int64) bool {
	if !f.Enabled {
		return false
	}
	if len(f.Rules.Users) == 0 && f.Rules.Ratio <= 0 {
		return true // 全量
	}
	for _, u := range f.Rules.Users {
		if u == userID {
			return true
		}
	}
	if f.Rules.Ratio >= 100 {
		return true
	}
	if f.Rules.Ratio > 0 {
		return bucket(f.Key, userID) < f.Rules.Ratio
	}
	return false
}

// bucket 稳定哈希分桶（0-99），保证同一用户粘性（不抖动）。
func bucket(key string, userID int64) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(userID >> (8 * i))
	}
	_, _ = h.Write(b[:])
	return int(h.Sum32() % 100)
}
