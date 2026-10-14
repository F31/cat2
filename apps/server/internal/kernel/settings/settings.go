// Package settings 业务配置：模块注册 SettingsSchema，后台据此自动生成配置界面；
// 带版本、生效时间、操作人审计；scope 支持 platform/space，取值顺序 space → platform。
package settings

import (
	"context"
	"fmt"
	"strconv"
)

// ValueType 配置值类型。
type ValueType string

const (
	TypeInt    ValueType = "int"
	TypeFen    ValueType = "fen"    // 金额分
	TypeRate   ValueType = "rate"   // 折扣系数万分比
	TypeBool   ValueType = "bool"
	TypeString ValueType = "string"
)

// Schema 配置项声明。
type Schema struct {
	Key         string
	Type        ValueType
	Default     string
	Description string
	Sensitive   bool // 敏感配置（值在后台脱敏展示）
}

// Entry 一条配置值。
type Entry struct {
	Scope    string // "platform" 或 "space:<id>"
	Key      string
	Value    string
	Version  int
	Operator int64
}

// Store 配置存储（sys_settings）。
type Store interface {
	Get(ctx context.Context, scope, key string) (Entry, bool, error)
	Set(ctx context.Context, e Entry) error
	List(ctx context.Context, scope string) ([]Entry, error)
}

// Registry 配置注册与读取。
type Registry struct {
	schemas map[string]Schema
	store   Store
}

func NewRegistry(store Store) *Registry {
	return &Registry{schemas: map[string]Schema{}, store: store}
}

func (r *Registry) Register(ss []Schema) {
	for _, s := range ss {
		r.schemas[s.Key] = s
	}
}

// Schemas 全部声明（后台自动生成配置界面）。
func (r *Registry) Schemas() []Schema {
	out := make([]Schema, 0, len(r.schemas))
	for _, s := range r.schemas {
		out = append(out, s)
	}
	return out
}

// Get 读取配置：space → platform → schema 默认值。
func (r *Registry) Get(ctx context.Context, spaceID int16, key string) (string, error) {
	if r.store != nil {
		if spaceID > 1 {
			if e, ok, err := r.store.Get(ctx, fmt.Sprintf("space:%d", spaceID), key); err != nil {
				return "", err
			} else if ok {
				return e.Value, nil
			}
		}
		if e, ok, err := r.store.Get(ctx, "platform", key); err != nil {
			return "", err
		} else if ok {
			return e.Value, nil
		}
	}
	if s, ok := r.schemas[key]; ok {
		return s.Default, nil
	}
	return "", fmt.Errorf("settings: 未登记的配置键 %q", key)
}

// Int 读取整数配置。
func (r *Registry) Int(ctx context.Context, spaceID int16, key string) (int64, error) {
	v, err := r.Get(ctx, spaceID, key)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(v, 10, 64)
}
