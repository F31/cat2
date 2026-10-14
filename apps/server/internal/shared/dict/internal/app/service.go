// Package app dict 应用服务：选项查询、TagSet 统一校验。
package app

import (
	"context"
	"fmt"

	"catcube/internal/shared/dict/contract"
)

// Repo 字典仓储接口（domain/app 定义，infra 实现）。
type Repo interface {
	Options(ctx context.Context, dimension string) ([]contract.Option, error)
	Resolve(ctx context.Context, code string) (contract.Option, error)
	SubjectTypes(ctx context.Context) ([]contract.SubjectType, error)
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

// FieldTemplate 画像字段模板（字段取自代码登记的字段池；模板逻辑后续完善）。
func (s *Service) FieldTemplate(_ context.Context, _ string) ([]string, error) {
	return nil, fmt.Errorf("dict: FieldTemplate 待 p2 实现")
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
