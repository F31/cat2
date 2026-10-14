// Package infra dict 仓储实现（PG，dct_ 前缀）。
package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"catcube/internal/kernel/db"
	"catcube/pkg/ids"
)

// SeedFile contracts/seeds/dict/*.json 的文件格式。
type SeedFile struct {
	Dimension string `json:"dimension"`
	Version   int    `json:"version"`
	Items     []struct {
		Code       string `json:"code"`
		Name       string `json:"name"`
		ParentCode string `json:"parent_code"`
		Sort       int    `json:"sort"`
	} `json:"items"`
}

// ImportSeeds 幂等导入目录下全部字典种子：按 code 冲突更新，只停用不删除。
// subject_type 维度入 dct_subject_type，其余入 dct_taxonomy_option。
// 返回导入条目总数。
func (r *Repo) ImportSeeds(ctx context.Context, dir string) (int, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return 0, err
	}
	sort.Strings(files)
	total := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return total, err
		}
		var sf SeedFile
		if err := json.Unmarshal(raw, &sf); err != nil {
			return total, fmt.Errorf("种子文件 %s 解析失败: %w", f, err)
		}
		if sf.Dimension == "" {
			return total, fmt.Errorf("种子文件 %s 缺 dimension", f)
		}
		n, err := r.importOne(ctx, sf)
		if err != nil {
			return total, fmt.Errorf("种子文件 %s 导入失败: %w", f, err)
		}
		total += n
	}
	return total, nil
}

func (r *Repo) importOne(ctx context.Context, sf SeedFile) (int, error) {
	n := 0
	err := r.pool.InTx(ctx, func(tx db.Tx) error {
		for _, it := range sf.Items {
			if it.Code == "" || it.Name == "" {
				return fmt.Errorf("维度 %s 存在缺 code/name 的条目", sf.Dimension)
			}
			var err error
			if sf.Dimension == "subject_type" {
				_, err = tx.Exec(ctx, `
					INSERT INTO dct_subject_type (id, code, name, sort, status, created_at, updated_at)
					VALUES ($1,$2,$3,$4,1,now(),now())
					ON CONFLICT (code) DO UPDATE SET name=EXCLUDED.name, sort=EXCLUDED.sort, updated_at=now()`,
					int64(ids.New()), it.Code, it.Name, it.Sort)
			} else {
				_, err = tx.Exec(ctx, `
					INSERT INTO dct_taxonomy_option (id, dimension, code, label, parent_code, sort, status, created_at, updated_at)
					VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,1,now(),now())
					ON CONFLICT (code) DO UPDATE SET label=EXCLUDED.label, parent_code=EXCLUDED.parent_code, sort=EXCLUDED.sort, updated_at=now()`,
					int64(ids.New()), sf.Dimension, it.Code, it.Name, it.ParentCode, it.Sort)
			}
			if err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}
