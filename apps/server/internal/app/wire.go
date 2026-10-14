// Package app 装配根：唯一同时 import 所有模块与适配器的地方。
// 读配置 → 选适配器 → 装载模块；S0→S1→S2 的差别只在这里与配置。
package app

import (
	"context"
	"fmt"
	"net/http"

	"catcube/internal/adapters/local"
	"catcube/internal/adapters/oss"
	"catcube/internal/adapters/pg"
	"catcube/internal/adapters/wechat"
	"catcube/internal/kernel/config"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/migrate"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/obs"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/scheduler"
	"catcube/internal/modules/identity"
	"catcube/internal/modules/trust"
	"catcube/internal/shared/dict"
	"catcube/internal/shared/media"
)

// App 装配结果。
type App struct {
	Config   config.Config
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Policy   *policy.Engine
	Modules  []module.Module
	Mux      *http.ServeMux
	Sched    *scheduler.Runner
	ready    bool
}

// Wire 装配。
func Wire(ctx context.Context, cfg config.Config) (*App, error) {
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	// 适配器选择（S0：pg 队列/搜索、local 缓存、wechat 登录骨架、oss 占位）
	queue := pg.NewQueue(pool)
	_ = queue
	cache := local.NewCache()
	_ = cache
	wechatAuth := wechat.NewAuthClient("", "") // appid/secret 经启动配置注入
	storage := oss.NewClient("", "default", "", "", "")

	// 内核机制
	outbox := pg.NewOutbox(pool)
	bus := eventbus.NewBus(outbox, pg.NewProcessed(pool))
	policyEngine := policy.NewEngine()

	// 模块（一期：shared + R0 骨架）
	trustMod := trust.New(trust.Deps{Pool: pool, Moderation: nil})
	identityMod := identity.New(identity.Deps{Pool: pool, WeChat: wechatAuth, Bus: bus})
	mods := []module.Module{
		dict.New(pool),
		media.New(pool, storage),
		identityMod,
		trustMod,
	}

	// 依赖拓扑装载：缺依赖启动即失败
	ordered, err := module.Load(mods)
	if err != nil {
		return nil, err
	}

	a := &App{Config: cfg, Pool: pool, Bus: bus, Policy: policyEngine, Modules: ordered, Mux: http.NewServeMux()}

	// 逐模块 Register：收集路由/事件/任务/能力
	for _, m := range ordered {
		reg := module.NewRegistry(m.Name())
		if err := m.Register(reg); err != nil {
			return nil, fmt.Errorf("wire: 模块 %s 注册失败: %w", m.Name(), err)
		}
		for _, rt := range reg.Routes {
			a.Mux.HandleFunc(rt.Method+" "+rt.Path, rt.Handler)
		}
		bus.Register(reg.EventSubs)
		policyEngine.RegisterProviders(reg.CapProviders)
		policyEngine.RegisterRules(reg.PolicyRules)
		a.scheduler().Register(toSchedulerJobs(reg.Jobs))
	}

	// 健康检查
	a.Mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	a.Mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil || !a.ready {
			http.Error(w, `{"status":"not_ready"}`, http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	// 事件订阅示例：identity.UserDeactivated → 各模块自行处理（p2 起逐模块补）
	return a, nil
}

func (a *App) scheduler() *scheduler.Runner {
	if a.Sched == nil {
		a.Sched = scheduler.NewRunner(a.Pool, 20261013)
	}
	return a.Sched
}

func toSchedulerJobs(jobs []module.JobDef) []scheduler.Job {
	out := make([]scheduler.Job, len(jobs))
	for i, j := range jobs {
		out[i] = scheduler.Job{Name: j.Name, Interval: j.Interval, Run: j.Run}
	}
	return out
}

// SeedDict 幂等导入字典种子（seed 子命令）。
func SeedDict(ctx context.Context, a *App, dir string) (int, error) {
	for _, m := range a.Modules {
		if d, ok := m.(*dict.Module); ok {
			return d.Seed(ctx, dir)
		}
	}
	return 0, fmt.Errorf("dict 模块未装载")
}

// RunMigrations 执行内核表 + 各模块迁移（按拓扑顺序）。
func RunMigrations(ctx context.Context, pool *db.Pool, mods []module.Module) error {
	if _, err := pool.Exec(ctx, pg.KernelTables()); err != nil {
		return fmt.Errorf("migrate: 内核表失败: %w", err)
	}
	type migrator interface{ Migrations() string }
	var sources []migrate.Source
	for _, m := range mods {
		if mm, ok := m.(migrator); ok {
			sources = append(sources, migrate.Source{
				Module:     m.Name(),
				Migrations: []migrate.Migration{{Version: 1, Name: "init", SQL: mm.Migrations()}},
			})
		}
	}
	return migrate.NewRunner(pool).Apply(ctx, sources)
}

// Serve 启动 HTTP（api/admin 角色）。
func (a *App) Serve(ctx context.Context) error {
	a.ready = true
	srv := &http.Server{Addr: a.Config.HTTPAddr, Handler: obs.RequestIDMiddleware(a.Mux)}
	go func() {
		<-ctx.Done()
		a.ready = false
		_ = srv.Shutdown(context.Background())
	}()
	obs.Log(ctx).Info("catcube 启动", "addr", a.Config.HTTPAddr, "roles", a.Config.Roles)
	return srv.ListenAndServe()
}
