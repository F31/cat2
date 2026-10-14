// Package dict 共享业务域模块：taxonomy 选项、主体类型配置、画像字段模板、TagSet 校验。
// 只依赖 L0/L1，不得依赖任何业务模块。
package dict

import (
	"context"
	"encoding/json"
	"net/http"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/module"
	"catcube/internal/shared/dict/contract"
	"catcube/internal/shared/dict/internal/app"
	"catcube/internal/shared/dict/internal/infra"
)

// Module dict 模块。
type Module struct {
	pool *db.Pool
	svc  *app.Service
}

// New 由装配根注入数据库池。
func New(pool *db.Pool) *Module { return &Module{pool: pool} }

func (m *Module) Name() string          { return "dict" }
func (m *Module) Rank() module.Rank     { return module.RankShared }
func (m *Module) DependsOn() []string   { return nil }

// Facade 对外契约（其他模块经此调用）。
func (m *Module) Facade() contract.Facade { return m.svc }

// Register 注册路由与配置。
func (m *Module) Register(r *module.Registry) error {
	m.svc = app.NewService(infra.NewRepo(m.pool))
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/dict/options", Handler: m.handleOptions},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/dict/subject-types", Handler: m.handleSubjectTypes},
	)
	return nil
}

// Migrations 本模块迁移（按 DependsOn 拓扑由内核迁移器执行）。
func (m *Module) Migrations() string { return migrations }

// Seed 幂等导入 contracts/seeds/dict 目录下的字典种子（seed 子命令调用）。
func (m *Module) Seed(ctx context.Context, dir string) (int, error) {
	return infra.NewRepo(m.pool).ImportSeeds(ctx, dir)
}

func (m *Module) handleOptions(w http.ResponseWriter, req *http.Request) {
	opts, err := m.svc.Options(req.Context(), req.URL.Query().Get("dimension"))
	writeJSON(w, opts, err)
}

func (m *Module) handleSubjectTypes(w http.ResponseWriter, req *http.Request) {
	ts, err := m.svc.SubjectTypes(req.Context())
	writeJSON(w, ts, err)
}

func writeJSON(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "服务错误"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}
