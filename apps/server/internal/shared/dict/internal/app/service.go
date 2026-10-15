// Package app dict 应用服务：选项查询、TagSet 统一校验。
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"catcube/internal/shared/dict/contract"
)

// Repo 字典仓储接口（domain/app 定义，infra 实现）。
type Repo interface {
	Options(ctx context.Context, dimension string) ([]contract.Option, error)
	Resolve(ctx context.Context, code string) (contract.Option, error)
	SubjectTypes(ctx context.Context) ([]contract.SubjectType, error)
	FieldDefs(ctx context.Context) ([]contract.FieldDef, error)
	// FieldTemplateFields 返回模板字段声明原始 JSON（[{field,required,sort}]），无模板返回 nil。
	FieldTemplateFields(ctx context.Context, subjectTypeCode string) ([]byte, error)
}

// Service dict 服务（实现 contract.Facade）。
type Service struct {
	repo Repo
}

func NewService(repo Repo) *Service { return &Service{repo: repo} }

func (s *Service) Options(ctx context.Context, dimension string) ([]contract.Option, error) {
	opts, err := s.repo.Options(ctx, dimension)
	if err != nil {
		return nil, err
	}
	out := opts[:0]
	for _, o := range opts {
		if o.Enabled {
			out = append(out, o)
		}
	}
	return out, nil
}

func (s *Service) Resolve(ctx context.Context, code string) (contract.Option, error) {
	return s.repo.Resolve(ctx, code)
}

func (s *Service) SubjectTypes(ctx context.Context) ([]contract.SubjectType, error) {
	return s.repo.SubjectTypes(ctx)
}

// FieldDefs 字段池。
func (s *Service) FieldDefs(ctx context.Context) ([]contract.FieldDef, error) {
	return s.repo.FieldDefs(ctx)
}

// FieldTemplate 画像字段模板：模板声明（[{field,required,sort}]）与字段池按 field 联表渲染。
func (s *Service) FieldTemplate(ctx context.Context, subjectTypeCode string) ([]contract.RenderedField, error) {
	raw, err := s.repo.FieldTemplateFields(ctx, subjectTypeCode)
	if err != nil {
		return nil, err
	}
	defs, err := s.repo.FieldDefs(ctx)
	if err != nil {
		return nil, err
	}
	byField := make(map[string]contract.FieldDef, len(defs))
	for _, d := range defs {
		byField[d.Field] = d
	}
	return renderTemplate(raw, byField)
}

type templateField struct {
	Field    string `json:"field"`
	Required bool   `json:"required"`
	Sort     int    `json:"sort"`
}

func renderTemplate(raw []byte, byField map[string]contract.FieldDef) ([]contract.RenderedField, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var tfs []templateField
	if err := json.Unmarshal(raw, &tfs); err != nil {
		return nil, fmt.Errorf("dict: 字段模板格式非法: %w", err)
	}
	out := make([]contract.RenderedField, 0, len(tfs))
	for _, t := range tfs {
		def, ok := byField[t.Field]
		if !ok {
			// 模板引用了未登记字段：显式报错，避免静默丢字段
			return nil, fmt.Errorf("dict: 模板字段 %q 未在字段池登记", t.Field)
		}
		out = append(out, contract.RenderedField{
			Field: def.Field, Label: def.Label, ValueType: def.ValueType, Required: t.Required, Sort: t.Sort,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sort < out[j].Sort })
	return out, nil
}

// RenderValidate 渲染模板并校验画像值：必填齐全、值类型合法。
func (s *Service) RenderValidate(ctx context.Context, subjectTypeCode string, values map[string]any) ([]contract.RenderedField, error) {
	fields, err := s.FieldTemplate(ctx, subjectTypeCode)
	if err != nil {
		return nil, err
	}
	for _, f := range fields {
		v, ok := values[f.Field]
		if !ok || v == nil || v == "" {
			if f.Required {
				return nil, fmt.Errorf("dict: 字段「%s」为必填", f.Label)
			}
			continue
		}
		if err := validateValue(f, v); err != nil {
			return nil, err
		}
	}
	return fields, nil
}

// validateValue 值类型校验（枚举/仅登记字段）。
func validateValue(f contract.RenderedField, v any) error {
	switch f.ValueType {
	case "int", "fen":
		switch n := v.(type) {
		case float64:
			if n != float64(int64(n)) {
				return fmt.Errorf("dict: 字段「%s」应为整数", f.Label)
			}
		case int, int64, json.Number:
		default:
			return fmt.Errorf("dict: 字段「%s」应为整数", f.Label)
		}
	case "bool":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("dict: 字段「%s」应为布尔", f.Label)
		}
	case "string", "text", "enum", "file", "phone":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("dict: 字段「%s」应为字符串", f.Label)
		}
	case "tags":
		if _, ok := v.([]any); !ok {
			if _, ok := v.([]string); !ok {
				return fmt.Errorf("dict: 字段「%s」应为数组", f.Label)
			}
		}
	}
	return nil
}

// ValidateTagSet 统一校验：所有 code 必须存在且启用。
func (s *Service) ValidateTagSet(ctx context.Context, ts contract.TagSet) error {
	check := func(dimension string, codes []string) error {
		for _, c := range codes {
			o, err := s.repo.Resolve(ctx, c)
			if err != nil {
				return fmt.Errorf("dict: %s 含非法 code %q", dimension, c)
			}
			if !o.Enabled {
				return fmt.Errorf("dict: %s 的 code %q 已停用", dimension, c)
			}
		}
		return nil
	}
	if err := check("industry", ts.Industry); err != nil {
		return err
	}
	if ts.Region != "" {
		if err := check("region", []string{ts.Region}); err != nil {
			return err
		}
	}
	if err := check("nature", ts.Nature); err != nil {
		return err
	}
	if err := check("need", ts.Need); err != nil {
		return err
	}
	if err := check("offer", ts.Offer); err != nil {
		return err
	}
	return check("domain", ts.Domain)
}

var _ contract.Facade = (*Service)(nil)
