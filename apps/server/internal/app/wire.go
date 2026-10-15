// Package app 装配根：唯一同时 import 所有模块与适配器的地方。
// 读配置 → 选适配器 → 装载模块；S0→S1→S2 的差别只在这里与配置。
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"catcube/internal/adapters/local"
	"catcube/internal/adapters/oss"
	"catcube/internal/adapters/pay"
	"catcube/internal/adapters/pg"
	"catcube/internal/adapters/realname"
	"catcube/internal/adapters/wechat"
	"catcube/internal/bff"
	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/config"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/migrate"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/obs"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/scheduler"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/kernel/settings"
	"catcube/internal/modules/circle"
	"catcube/internal/modules/content"
	contentcontract "catcube/internal/modules/content/contract"
	"catcube/internal/modules/cube"
	"catcube/internal/modules/discovery"
	"catcube/internal/modules/identity"
	"catcube/internal/modules/membership"
	"catcube/internal/modules/msg"
	"catcube/internal/modules/notice"
	"catcube/internal/modules/ops"
	"catcube/internal/modules/profile"
	"catcube/internal/modules/social"
	"catcube/internal/modules/trade"
	"catcube/internal/modules/trust"
	"catcube/internal/modules/verify"
	"catcube/internal/shared/dict"
	"catcube/internal/shared/entityref"
	"catcube/internal/shared/media"
)

// decodeHexKey 解析 hex 密钥；未配置时使用开发派生密钥并告警（生产必须配置）。
func decodeHexKey(hexKey, devSeed string) []byte {
	if hexKey != "" {
		if b, err := hex.DecodeString(hexKey); err == nil && len(b) == 32 {
			return b
		}
		slog.Warn("密钥格式非法（需 64 位十六进制），已回退开发密钥")
	} else {
		slog.Warn("未配置密钥，使用开发派生密钥（不可用于生产）")
	}
	sum := sha256.Sum256([]byte(devSeed))
	return sum[:]
}

// App 装配结果。
type App struct {
	Config  config.Config
	Pool    *db.Pool
	Bus     *eventbus.Bus
	Policy  *policy.Engine
	Modules []module.Module
	Mux     *http.ServeMux
	Sched   *scheduler.Runner
	ready   bool
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
	// 敏感数据密钥：生产必配（缺失时开发态告警并使用派生密钥）
	encKey := decodeHexKey(cfg.IdentityEncKey, "catcube-dev-enc-key-32bytes!!")
	blindKey := decodeHexKey(cfg.BlindKey, "catcube-dev-blind-key-32bytes")
	encryptor, err := sensitive.NewEncryptor(encKey)
	if err != nil {
		return nil, err
	}

	// 内核机制
	outbox := pg.NewOutbox(pool)
	bus := eventbus.NewBus(outbox, pg.NewProcessed(pool))
	policyEngine := policy.NewEngine()
	auditRec := audit.NewRecorder(pg.NewAudit(pool))

	// 微信：配了 appid/secret 走真实接口，否则开发态假实现（登录链路可端到端跑通）
	identityDeps := identity.Deps{
		Pool: pool, Bus: bus, Encryptor: encryptor, BlindKey: blindKey, DevSMSCode: cfg.DevSMSCode,
		Audit: auditRec,
	}
	if cfg.WeChatAppID != "" && cfg.WeChatAppSecret != "" {
		authClient := wechat.NewAuthClient(cfg.WeChatAppID, cfg.WeChatAppSecret)
		identityDeps.WeChat = authClient
		identityDeps.PhoneCode = authClient
	} else {
		dev := wechat.NewDevAuthClient()
		slog.Info("微信未配置 appid/secret：使用开发态登录实现（不可用于生产）")
		identityDeps.WeChat = dev
		identityDeps.PhoneCode = dev
	}
	storage := oss.NewClient("", "default", "", "", "")

	// 模块（shared + R0 + R1 + R2）
	identityMod := identity.New(identityDeps)
	// 鉴权函数：供 R0 trust 等无法依赖 identity 的同阶模块复用（经装配根注入）
	authFunc := func(ctx context.Context, token string) (int64, error) {
		uid, ver, err := identityMod.Facade().Authenticate(ctx, token)
		if err != nil {
			return 0, err
		}
		b, err := identityMod.Facade().UserBrief(ctx, uid)
		if err != nil || b.AuthzVer != ver || (b.Status != 0 && b.Status != 1) {
			return 0, fmt.Errorf("identity: 令牌已失效")
		}
		return uid, nil
	}
	trustMod := trust.New(trust.Deps{Pool: pool, Moderation: nil, Bus: bus, Auth: authFunc, Audit: auditRec})
	dictMod := dict.New(pool)
	mediaMod := media.New(pool, storage)
	entityReg := entityref.NewRegistry()
	profileMod := profile.New(profile.Deps{
		Pool: pool, Bus: bus, Media: mediaMod.Facade(), Dict: dictMod.Facade(),
		Identity: identityMod.Facade(), Encryptor: encryptor, EntityRegistry: entityReg,
	})
	verifyMod := verify.New(verify.Deps{
		Pool: pool, Bus: bus, Verify: realname.NewDevClient(),
		Identity: identityMod.Facade(), Encryptor: encryptor,
	})
	contentMod := content.New(content.Deps{
		Pool: pool, Bus: bus, Dict: dictMod.Facade(), Trust: trustMod.Facade(), Media: mediaMod.Facade(),
		Identity: identityMod.Facade(), Policy: policyEngine, Encryptor: encryptor, EntityRegistry: entityReg,
		Audit: auditRec,
	})
	socialMod := social.New(social.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Resolver: entityReg,
	})
	msgMod := msg.New(msg.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Policy: policyEngine,
		Resolver: entityReg, Realtime: local.RealtimeNoop{}, Audit: auditRec, Storage: storage,
	})
	settingsReg := settings.NewRegistry(nil)
	discoveryMod := discovery.New(discovery.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Settings: settingsReg, Outbox: outbox,
	})
	tradeMod := trade.New(trade.Deps{
		Pool: pool, Bus: bus, Pay: pay.NewDevClient(), Identity: identityMod.Facade(), Audit: auditRec,
	})
	membershipMod := membership.New(membership.Deps{
		Pool: pool, Bus: bus, Trade: tradeMod.Facade(), Verify: verifyMod.Facade(),
		Identity: identityMod.Facade(), Settings: settingsReg, Policy: policyEngine,
	})
	cubeMod := cube.New(cube.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Policy: policyEngine, Entities: entityReg,
		Audit: auditRec, Encryptor: encryptor, Trade: tradeMod.Facade(), Settings: settingsReg,
		Content: contentMod.Facade(), Msg: msgMod.Facade(),
	})
	circleMod := circle.New(circle.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Policy: policyEngine,
		Encryptor: encryptor, Entities: entityReg, Content: contentMod.Facade(), Cube: cubeMod.Facade(),
	})
	// cube 作为群成员解析器注册给 msg（msg 不认识猫立方）
	msgMod.SetMemberResolver(cubeMod.SessionMemberResolver())
	noticeMod := notice.New(notice.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Circle: circleMod.Facade(),
	})
	opsMod := ops.New(ops.Deps{Pool: pool, Identity: identityMod.Facade(), Outbox: outbox, Bus: bus})
	mods := []module.Module{
		dictMod,
		mediaMod,
		opsMod,
		identityMod,
		trustMod,
		profileMod,
		verifyMod,
		contentMod,
		socialMod,
		tradeMod,
		membershipMod,
		cubeMod,
		circleMod,
		noticeMod,
		msgMod,
		discoveryMod,
	}

	// 模块开关（--modules / CATCUBE_MODULES）：空=全部；共享域与 ops 始终装载
	selected, err := selectModules(mods, cfg.Modules)
	if err != nil {
		return nil, err
	}
	mods = selected

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
		settingsReg.Register(reg.Settings)
		a.scheduler().Register(toSchedulerJobs(reg.Jobs))
	}

	// 履约能力注册（装配根唯一装配扩展点）：会员开通/续期由 trade 支付成功回调触发
	tradeMod.RegisterFulfiller(membershipMod)
	tradeMod.RegisterFulfiller(cubeMod)

	// 敏感词库加载（后台可维护）：启动后周期刷新，保证机检词库生效
	_ = trustMod.LoadWords(context.Background())
	a.scheduler().Register([]scheduler.Job{{
		Name: "trust.load_words", Interval: time.Minute,
		Run: func(ctx context.Context) error { return trustMod.LoadWords(ctx) },
	}})

	// 事件投递 worker：轮询 Outbox 分发订阅（至少一次；订阅方幂等）
	a.scheduler().Register([]scheduler.Job{{
		Name: "kernel.outbox_relay", Interval: 2 * time.Second,
		Run: func(ctx context.Context) error {
			_, err := bus.Relay(ctx, "worker", 200)
			return err
		},
	}})

	// BFF 只读聚合层（组合各模块 contract）：首屏聚合与详情聚合
	bff.New(bff.Deps{
		Identity: identityMod.Facade(), Discovery: discoveryMod.Facade(), Profile: profileMod.Facade(),
		Social: socialMod.Facade(), Msg: msgMod.Facade(), Verify: verifyMod.Facade(),
		Membership: membershipMod.Facade(), Content: contentMod.Facade(),
		Cube: cubeMod.Facade(), Circle: circleMod.Facade(),
	}).Register(a.Mux)

	// 跨模块扩展点装配（装配根唯一 import 所有模块）：审核对象提供者
	trustMod.RegisterSubjectProvider(profileMod)
	trustMod.RegisterSubjectProvider(verifyMod)
	trustMod.RegisterSubjectProvider(msgMod.ProviderFor())
	trustMod.RegisterSubjectProvider(circleMod.ProviderFor())
	for _, k := range []string{contentcontract.KindProject, contentcontract.KindProduct, contentcontract.KindMeeting, contentcontract.KindEvent} {
		trustMod.RegisterSubjectProvider(contentMod.ProviderFor(k))
	}

	// 后台菜单声明（p1 §9.9/§11.7）：模块 → AdminSection 声明式；p4 扩为菜单级/操作级权限点
	adminMenus := adminSectionsFor(ordered)
	a.Mux.HandleFunc("GET /api/admin/v1/menus", func(w http.ResponseWriter, req *http.Request) {
		token := ""
		if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		}
		if token == "" {
			http.Error(w, `{"code":2,"message":"未登录"}`, http.StatusUnauthorized)
			return
		}
		if _, _, err := identityMod.Facade().Authenticate(req.Context(), token); err != nil {
			http.Error(w, `{"code":2,"message":"登录已失效"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": map[string]any{"items": adminMenus}})
	})

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

// adminSectionsFor 后台菜单声明（装配根集中声明，p4 改为模块 AdminSection + 操作级权限点）。
func adminSectionsFor(mods []module.Module) []module.AdminSection {
	decl := map[string][]module.AdminSection{
		"identity":   {{Key: "identity.users", Label: "用户管理", Path: "/users", Module: "identity"}},
		"content":    {{Key: "content.contents", Label: "内容管理", Path: "/contents", Module: "content"}},
		"trust":      {{Key: "trust.reviews", Label: "审核队列", Path: "/reviews", Module: "trust"}},
		"trade":      {{Key: "trade.orders", Label: "订单管理", Path: "/orders", Module: "trade"}},
		"membership": {{Key: "membership.members", Label: "会员管理", Path: "/members", Module: "membership"}},
		"cube":       {{Key: "cube.cubes", Label: "猫立方管理", Path: "/cubes", Module: "cube"}},
		"circle":     {{Key: "circle.circles", Label: "圈子管理", Path: "/circles", Module: "circle"}},
		"profile":    {{Key: "profile.templates", Label: "画像字段模板", Path: "/profile-templates", Module: "profile"}},
		"notice":     {{Key: "notice.templates", Label: "通知模板", Path: "/templates", Module: "notice"}},
		"ops":        {{Key: "ops.dead_letters", Label: "死信队列", Path: "/dead-letters", Module: "ops"}},
		"dict":       {{Key: "dict.taxonomy", Label: "主体类型与选项", Path: "/dict", Module: "dict"}},
	}
	var out []module.AdminSection
	for _, m := range mods {
		out = append(out, decl[m.Name()]...)
	}
	return out
}

// selectModules 按 --modules 过滤（空=全部）：共享域与 ops 始终装载；
// 被启用模块的依赖自动向上补齐（传递闭包），未知模块名报错。
func selectModules(all []module.Module, enabled []string) ([]module.Module, error) {
	if len(enabled) == 0 {
		return all, nil
	}
	byName := map[string]module.Module{}
	for _, m := range all {
		byName[m.Name()] = m
	}
	on := map[string]bool{}
	for _, n := range enabled {
		if _, ok := byName[n]; !ok {
			return nil, fmt.Errorf("modules: 未知模块 %q", n)
		}
		on[n] = true
	}
	// 传递补齐依赖
	changed := true
	for changed {
		changed = false
		for name := range on {
			for _, dep := range byName[name].DependsOn() {
				if _, ok := byName[dep]; !ok {
					return nil, fmt.Errorf("modules: %q 依赖缺失的 %q", name, dep)
				}
				if !on[dep] {
					on[dep] = true
					changed = true
				}
			}
		}
	}
	var out []module.Module
	for _, m := range all {
		if m.Rank() == module.RankShared || m.Name() == "ops" || on[m.Name()] {
			out = append(out, m)
		}
	}
	return out, nil
}

// RunMigrations 执行内核表 + 各模块迁移（按拓扑顺序）。
func RunMigrations(ctx context.Context, pool *db.Pool, mods []module.Module) error {
	if _, err := pool.Exec(ctx, pg.KernelTables()); err != nil {
		return fmt.Errorf("migrate: 内核表失败: %w", err)
	}
	// 每模块迁移以 SQL 内容哈希为版本：脚本为幂等 DDL（IF NOT EXISTS），
	// 内容变更即重新应用，避免"新增表被 version=1 已应用而跳过"。
	type migrator interface{ Migrations() string }
	var sources []migrate.Source
	for _, m := range mods {
		if mm, ok := m.(migrator); ok {
			sql := mm.Migrations()
			if strings.TrimSpace(sql) == "" {
				continue // 无自有表（如 ops）
			}
			version := int(crc32.ChecksumIEEE([]byte(sql)) & 0x7fffffff) // 收敛到 int4 正区间
			sources = append(sources, migrate.Source{
				Module:     m.Name(),
				Migrations: []migrate.Migration{{Version: version, Name: "init", SQL: sql}},
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
