// Package contract dict 对外契约：taxonomy 选项、主体类型配置、画像字段模板、TagSet 校验。
package contract

import "context"

// Option 字典选项（只停用不删除）。
type Option struct {
	Dimension  string `json:"dimension"` // industry/sub_industry/region/nature/need/offer/domain...
	Code       string `json:"code"`
	Label      string `json:"label"`
	ParentCode string `json:"parent_code,omitempty"`
	Sort       int    `json:"sort"`
	Enabled    bool   `json:"enabled"`
}

// SubjectType 主体类型配置（编码创建后不可改）。
type SubjectType struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	IconFileID  int64  `json:"icon_file_id,string,omitempty"`
	CanPublish  bool   `json:"can_publish"`
	HomeVisible bool   `json:"home_visible"`
	Sort        int    `json:"sort"`
	Enabled     bool   `json:"enabled"`
}

// TagSet 通用标签值对象：所有可发布/可检索实体共用（存 code）。
type TagSet struct {
	Industry []string `json:"industry,omitempty"`
	Region   string   `json:"region,omitempty"`
	Nature   []string `json:"nature,omitempty"`
	Need     []string `json:"need,omitempty"`
	Offer    []string `json:"offer,omitempty"`
	Domain   []string `json:"domain,omitempty"`
}

// FieldDef 字段池定义（代码侧登记，随版本发布）。
type FieldDef struct {
	Field     string `json:"field"`
	Label     string `json:"label"`
	ValueType string `json:"value_type"` // string/text/int/fen/rate/bool/enum/tags/file/phone
}

// RenderedField 按主体类型模板渲染后的字段（含必填与排序）。
type RenderedField struct {
	Field     string `json:"field"`
	Label     string `json:"label"`
	ValueType string `json:"value_type"`
	Required  bool   `json:"required"`
	Sort      int    `json:"sort"`
}

// Facade dict 对外接口。
type Facade interface {
	Options(ctx context.Context, dimension string) ([]Option, error)
	Resolve(ctx context.Context, code string) (Option, error)
	ValidateTagSet(ctx context.Context, ts TagSet) error
	SubjectTypes(ctx context.Context) ([]SubjectType, error)
	FieldTemplate(ctx context.Context, subjectTypeCode string) ([]RenderedField, error)
	// RenderValidate 按主体类型模板渲染并校验画像值（必填齐全 + 类型合法）。
	RenderValidate(ctx context.Context, subjectTypeCode string, values map[string]any) ([]RenderedField, error)
	// FieldDefs 字段池（后台字段模板维护用）。
	FieldDefs(ctx context.Context) ([]FieldDef, error)
}
