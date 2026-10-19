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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"catcube/internal/adapters/llm"
	"catcube/internal/adapters/local"
	"catcube/internal/adapters/moderation"
	"catcube/internal/adapters/oss"
	"catcube/internal/adapters/pay"
	"catcube/internal/adapters/pg"
	"catcube/internal/adapters/realname"
	"catcube/internal/adapters/redis"
	"catcube/internal/adapters/wechat"
	"catcube/internal/bff"
	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/config"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/featureflag"
	"catcube/internal/kernel/menu"
	"catcube/internal/kernel/migrate"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/obs"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/rbac"
	"catcube/internal/kernel/scheduler"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/kernel/settings"
	"catcube/internal/kernel/statemachine"
	"catcube/internal/modules/ai"
	"catcube/internal/modules/channel"
	"catcube/internal/modules/circle"
	"catcube/internal/modules/content"
	contentcontract "catcube/internal/modules/content/contract"
	"catcube/internal/modules/cube"
	"catcube/internal/modules/discovery"
	"catcube/internal/modules/identity"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/modules/membership"
	"catcube/internal/modules/msg"
	"catcube/internal/modules/notice"
	"catcube/internal/modules/ops"
	"catcube/internal/modules/profile"
	"catcube/internal/modules/social"
	"catcube/internal/modules/trade"
	"catcube/internal/modules/trust"
	"catcube/internal/modules/verify"
	"catcube/internal/ports"
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
	Cache   ports.Cache              // 缓存（Redis 主备或进程内），供降级观测与测试
	AlertTh eventbus.AlertThresholds // Outbox 告警阈值（测试/看板）
	// StateMachines 全模块注册的状态机（表驱动自检，p1 §13.2）
	StateMachines []statemachine.Def
	// AuditPoints 全模块声明的审计点（敏感动词路由覆盖校验，p4 §5.3）
	AuditPoints []audit.Point
	// Audit 审计写入器（测试/治理工具复用）
	Audit *audit.Recorder
	// RBAC 后台角色权限（p4 §3，ADR-002）
	RBAC  *rbac.Store
	ready atomic.Bool // 就绪（停机时置 false，负载探针据此摘流量）
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
	// 缓存：配 Redis 时主备（Redis 失败回退进程内，p6 §4.4）；否则纯进程内（S0）
	memCache := local.NewCache()
	var cache ports.Cache = memCache
	if cfg.RedisAddr != "" {
		rc := redis.New(cfg.RedisAddr, cfg.RedisPassword)
		_ = rc.Ping(ctx) // 启动探测，失败即降级（不阻断启动）
		cache = redis.NewFallback(rc, memCache, func(err error) {
			slog.Warn("缓存降级至进程内", "err", err)
		})
	}
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
	auditStore := pg.NewAudit(pool)
	audit.SetMetrics(obs.Default())
	auditRec := audit.NewRecorder(auditStore)

	// 微信：配了 appid/secret 走真实接口，否则开发态假实现（登录链路可端到端跑通）
	settingsReg := settings.NewRegistry(pg.NewSettings(pool))
	// 双人复核清单来源（p4 §3.12）：从 settings 读取 `policy.dual_control`
	policyEngine.SetDualControlSource(func(ctx context.Context) (string, bool) {
		v, err := settingsReg.Get(ctx, 1, "policy.dual_control")
		if err != nil {
			return "", false
		}
		return v, true
	})
	identityDeps := identity.Deps{
		Pool: pool, Bus: bus, Encryptor: encryptor, BlindKey: blindKey, DevSMSCode: cfg.DevSMSCode,
		Audit: auditRec, Settings: settingsReg,
	}
	// 小程序接入信息优先从后台「小程序设置」读取（super admin 可改，热生效）；
	// 未配置时回退环境变量，仍为空则使用开发态实现。
	wechatClient := wechat.NewDynamicClient(settingsReg, cfg.WeChatAppID, cfg.WeChatAppSecret)
	identityDeps.WeChat = wechatClient
	identityDeps.PhoneCode = wechatClient
	var miniCoder ports.MiniProgramCoder = wechatClient
	storage := oss.NewClient(cfg.OSSEndpoint, cfg.OSSBucket, cfg.CDNHost, cfg.OSSAK, cfg.OSSSK)

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
	var moderationClient ports.Moderation
	if cfg.Moderation != "" {
		moderationClient = moderation.NewDevClient() // dev/演练实现（生产接微信内容安全适配器）
		slog.Info("内容安全启用开发实现", "mode", cfg.Moderation)
	}
	trustMod := trust.New(trust.Deps{Pool: pool, Moderation: moderationClient, Bus: bus, Auth: authFunc, Audit: auditRec,
		AlertReviewBacklog: cfg.AlertReviewBacklog})
	dictMod := dict.New(pool)
	mediaMod := media.New(pool, storage)
	entityReg := entityref.NewRegistry()
	profileMod := profile.New(profile.Deps{
		Pool: pool, Bus: bus, Media: mediaMod.Facade(), Dict: dictMod.Facade(),
		Identity: identityMod.Facade(), Encryptor: encryptor, EntityRegistry: entityReg,
		Audit: auditRec,
	})
	verifyMod := verify.New(verify.Deps{
		Pool: pool, Bus: bus, Verify: realname.NewDevClient(),
		Identity: identityMod.Facade(), Encryptor: encryptor, Audit: auditRec,
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
		Settings: settingsReg,
	})
	discoveryMod := discovery.New(discovery.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Settings: settingsReg, Outbox: outbox,
	})
	tradeMod := trade.New(trade.Deps{
		Pool: pool, Bus: bus, Pay: pay.NewDevClient(), Identity: identityMod.Facade(), Audit: auditRec,
		AlertRetryThreshold: cfg.AlertFulfillRetry,
	})
	membershipMod := membership.New(membership.Deps{
		Pool: pool, Bus: bus, Trade: tradeMod.Facade(), Verify: verifyMod.Facade(),
		Identity: identityMod.Facade(), Settings: settingsReg, Policy: policyEngine, Audit: auditRec,
	})
	cubeMod := cube.New(cube.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Policy: policyEngine, Entities: entityReg,
		Audit: auditRec, Encryptor: encryptor, Trade: tradeMod.Facade(), Settings: settingsReg,
		Content: contentMod.Facade(), Msg: msgMod.Facade(),
	})
	circleMod := circle.New(circle.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Policy: policyEngine,
		Encryptor: encryptor, Entities: entityReg, Content: contentMod.Facade(), Cube: cubeMod.Facade(),
		Audit: auditRec, Coder: miniCoder,
	})
	// cube 作为群成员解析器注册给 msg（msg 不认识猫立方）
	msgMod.SetMemberResolver(cubeMod.SessionMemberResolver())
	noticeMod := notice.New(notice.Deps{
		Pool: pool, Bus: bus, Identity: identityMod.Facade(), Circle: circleMod.Facade(), Audit: auditRec,
	})
	flagStore := featureflag.NewStore(pool)
	schedRunner := scheduler.NewRunner(pool, 20261013)
	rbacStore := rbac.New(pool)
	opsMod := ops.New(ops.Deps{Pool: pool, Identity: identityMod.Facade(), Outbox: outbox, Bus: bus,
		Flags: flagStore, Settings: settingsReg, Storage: storage, Audit: auditRec, Runner: schedRunner, RBAC: rbacStore})
	aiMod := ai.New(ai.Deps{Pool: pool, Flags: flagStore, LLM: llm.NewDevClient(), Auth: authFunc,
		Settings: settingsReg, Dict: dictMod.Facade(), Audit: auditRec})
	channelMod := channel.New(channel.Deps{Pool: pool, Bus: bus, Auth: authFunc, Audit: auditRec, Dict: dictMod.Facade(), Cube: cubeMod.Facade(), Settings: settingsReg, Policy: policyEngine})
	// 审核辅助（AI 预分类/摘要）：装配根桥接同阶模块（trust ← ai）
	trustMod.SetSuggester(aiSuggester{ai: aiMod.Facade()})
	mods := []module.Module{
		dictMod,
		mediaMod,
		opsMod,
		identityMod,
		trustMod,
		aiMod,
		profileMod,
		verifyMod,
		contentMod,
		socialMod,
		tradeMod,
		membershipMod,
		cubeMod,
		circleMod,
		channelMod,
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

	a := &App{Config: cfg, Pool: pool, Bus: bus, Policy: policyEngine, Modules: ordered, Mux: http.NewServeMux(), Sched: schedRunner, Cache: cache, Audit: auditRec, RBAC: rbacStore, AlertTh: eventbus.AlertThresholds{Backlog: cfg.AlertOutboxBacklog, OldestAge: cfg.AlertOutboxOldestSec, DeadCount: cfg.AlertOutboxDead}}

	// 逐模块 Register：收集路由/事件/任务/能力
	for _, m := range ordered {
		reg := module.NewRegistry(m.Name())
		if err := m.Register(reg); err != nil {
			return nil, fmt.Errorf("wire: 模块 %s 注册失败: %w", m.Name(), err)
		}
		a.StateMachines = append(a.StateMachines, reg.StateMachines...)
		a.AuditPoints = append(a.AuditPoints, reg.AuditPoints...)
		for _, rt := range reg.Routes {
			h := rt.Handler
			if rt.Admin {
				if rt.Perm != "" {
					h = requirePerm(a.RBAC, a.Audit, rt.Perm, h)
				}
				h = requireAdmin(identityMod.Facade(), a.RBAC, settingsReg, a.Audit, h) // 后台路由硬门槛（超管或任一角色）
			}
			a.Mux.HandleFunc(rt.Method+" "+rt.Path, h)
		}
		bus.Register(reg.EventSubs)
		policyEngine.RegisterProviders(reg.CapProviders)
		policyEngine.RegisterRules(reg.PolicyRules)
		settingsReg.Register(reg.Settings)
		a.scheduler().Register(toSchedulerJobs(reg.Jobs))
	}

	// 后台治理（RBAC）审计点（内核接口，非模块路由；p4 §5.4 admin/内核）
	a.AuditPoints = append(a.AuditPoints,
		audit.Point{Action: "admin:login:password", Path: "/api/admin/v1/auth/password", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "admin:password:change", Path: "/api/admin/v1/auth/password/change", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "admin:password:reset:code", Path: "/api/admin/v1/auth/password/reset/code", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "admin:password:reset", Path: "/api/admin/v1/auth/password/reset", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "ops:rbac:role:set", Path: "/api/admin/v1/roles/{code}/permissions", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "ops:rbac:role:scope", Path: "/api/admin/v1/roles/{code}/scope", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "ops:rbac:role:create", Path: "/api/admin/v1/roles", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "ops:rbac:role:delete", Path: "/api/admin/v1/roles/{code}", Required: true, LogFailed: true, Module: "ops"},
		audit.Point{Action: "ops:rbac:assign", Path: "/api/admin/v1/admin-users/{id}/roles", Required: true, LogFailed: true, Module: "ops"},
	)

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
	alertTh := eventbus.AlertThresholds{Backlog: cfg.AlertOutboxBacklog, OldestAge: cfg.AlertOutboxOldestSec, DeadCount: cfg.AlertOutboxDead}
	a.scheduler().Register([]scheduler.Job{
		{
			Name: "kernel.outbox_relay", Interval: 2 * time.Second,
			Run: func(ctx context.Context) error {
				_, err := bus.Relay(ctx, "worker", 200)
				return err
			},
		},
		{
			Name: "kernel.audit_verify", Interval: time.Hour,
			Run: func(ctx context.Context) error {
				// 审计哈希链增量断链/篡改检测告警（p6 §1.6）：自上次校验游标起，
				// 逐条重算 hash 并校验链条连接；篡改/插入/断链即告警。历史全量离线校验用 audit.VerifyChain。
				curID, curHash, err := auditStore.Cursor(ctx)
				if err != nil {
					return err
				}
				if curID == 0 {
					// 首次运行：以当前链尾为基线（不回溯历史，避免历史遗留误报）
					id, h, err := auditStore.Latest(ctx)
					if err != nil {
						return err
					}
					if id == 0 {
						obs.Default().SetGauge("catcube_audit_chain_ok", 1)
						return nil
					}
					obs.Default().SetGauge("catcube_audit_chain_ok", 1)
					return auditStore.SetCursor(ctx, id, h)
				}
				idx, msg, newID, newHash, err := audit.VerifyIncremental(ctx, auditStore, curID, curHash, 5000)
				if err != nil {
					return err
				}
				if idx > 0 {
					obs.Default().SetGauge("catcube_audit_chain_ok", 0)
					obs.Default().Alert("audit_chain_broken", fmt.Sprintf("%s（游标 id=%d）", msg, curID))
					return nil
				}
				obs.Default().SetGauge("catcube_audit_chain_ok", 1)
				if newID != curID {
					return auditStore.SetCursor(ctx, newID, newHash)
				}
				return nil
			},
		},
		{
			Name: "kernel.outbox_monitor", Interval: 30 * time.Second,
			Run: func(ctx context.Context) error {
				_, err := bus.ReportStats(ctx, alertTh)
				return err
			},
		},
	})

	// BFF 只读聚合层（组合各模块 contract）：首屏聚合与详情聚合
	bff.New(bff.Deps{
		Identity: identityMod.Facade(), Discovery: discoveryMod.Facade(), Profile: profileMod.Facade(),
		Social: socialMod.Facade(), Msg: msgMod.Facade(), Verify: verifyMod.Facade(),
		Membership: membershipMod.Facade(), Content: contentMod.Facade(),
		Cube: cubeMod.Facade(), Circle: circleMod.Facade(), Trust: trustMod.Facade(), Cache: cache,
		ContactDailyLimit: cfg.ContactDailyLimit, Audit: auditRec,
	}).Register(a.Mux)

	// 跨模块扩展点装配（装配根唯一 import 所有模块）：审核对象提供者
	trustMod.RegisterSubjectProvider(profileMod)
	trustMod.RegisterSubjectProvider(verifyMod)
	trustMod.RegisterSubjectProvider(msgMod.ProviderFor())
	trustMod.RegisterSubjectProvider(circleMod.ProviderFor())
	for _, k := range []string{contentcontract.KindProject, contentcontract.KindProduct, contentcontract.KindMeeting, contentcontract.KindEvent} {
		trustMod.RegisterSubjectProvider(contentMod.ProviderFor(k))
	}

	// 后台菜单（DB 驱动）：默认返回全部可见树；?group=xxx 仅返回该一级分组。
	menuStore := menu.New(pool)
	menusHandler := func(w http.ResponseWriter, req *http.Request) {
		uid, _ := adminUID(req)
		isSuper, _ := a.RBAC.IsSuper(req.Context(), uid)
		all, err := menuStore.List(req.Context(), "")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 9, "message": "菜单加载失败", "data": nil})
			return
		}
		visible := make([]menu.Item, 0, len(all))
		if isSuper {
			visible = all
		} else {
			perms, _ := a.RBAC.Perms(req.Context(), uid)
			mods := rbac.VisibleModules(perms)
			permSet := map[string]bool{}
			for _, p := range perms {
				permSet[p] = true
			}
			for _, it := range all {
				if it.SuperOnly {
					continue
				}
				if it.RequiredPerm != "" && !permSet[it.RequiredPerm] {
					continue
				}
				// 分类节点（/m/ 占位路径）不按模块过滤，由子节点决定是否展示
				if !strings.HasPrefix(it.Path, "/m/") && !mods[it.Module] {
					continue
				}
				visible = append(visible, it)
			}
		}
		// 剪掉没有可见子节点的分类
		hasChild := map[string]bool{}
		for _, it := range visible {
			if it.ParentKey != "" {
				hasChild[it.ParentKey] = true
			}
		}
		kept := make([]menu.Item, 0, len(visible))
		for _, it := range visible {
			if strings.HasPrefix(it.Path, "/m/") && !hasChild[it.Key] {
				continue
			}
			kept = append(kept, it)
		}
		// 一级分组（按可见菜单去重，保持顺序）
		type groupInfo struct {
			Key   string `json:"key"`
			Label string `json:"label"`
			Order int    `json:"order"`
		}
		groups := make([]groupInfo, 0, 8)
		seen := map[string]bool{}
		for _, it := range kept {
			if seen[it.Group] {
				continue
			}
			seen[it.Group] = true
			groups = append(groups, groupInfo{Key: it.Group, Label: it.GroupLabel, Order: it.GroupOrder})
		}
		items := kept
		if g := strings.TrimSpace(req.URL.Query().Get("group")); g != "" {
			filtered := make([]menu.Item, 0, len(kept))
			for _, it := range kept {
				if it.Group == g {
					filtered = append(filtered, it)
				}
			}
			items = filtered
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok",
			"data": map[string]any{"items": items, "groups": groups}})
	}
	a.Mux.HandleFunc("GET /api/admin/v1/menus", requireAdmin(identityMod.Facade(), a.RBAC, settingsReg, a.Audit, menusHandler))

	// ---- 后台 RBAC 配置（p4 §3，ADR-002）----
	adminJSON := func(w http.ResponseWriter, code int, msg string, data any) {
		w.Header().Set("Content-Type", "application/json")
		if code != 0 {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": msg, "data": data})
	}
	auditRBAC := func(req *http.Request, uid int64, action string, detail map[string]any, result audit.Result) {
		if a.Audit == nil {
			return
		}
		_ = a.Audit.Record(req.Context(), nil, audit.Entry{
			ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: action,
			Detail: detail, Result: result, IP: clientIP(req), RequestID: obs.RequestID(req.Context()),
		})
	}
	editorOf := func(req *http.Request) (int64, bool, []string) {
		uid, _ := adminUID(req)
		super, _ := a.RBAC.IsSuper(req.Context(), uid)
		perms, _ := a.RBAC.Perms(req.Context(), uid)
		return uid, super, perms
	}
	rbacAdmin := func(next http.HandlerFunc) http.HandlerFunc {
		return requireAdmin(identityMod.Facade(), a.RBAC, settingsReg, a.Audit, next)
	}
	rbacWrite := func(next http.HandlerFunc) http.HandlerFunc {
		return rbacAdmin(requirePerm(a.RBAC, a.Audit, rbac.PermRBACManage, next))
	}
	a.Mux.HandleFunc("GET /api/admin/v1/me", rbacAdmin(func(w http.ResponseWriter, req *http.Request) {
		uid, _ := adminUID(req)
		brief, _ := identityMod.Facade().UserBrief(req.Context(), uid)
		super, _ := a.RBAC.IsSuper(req.Context(), uid)
		codes, _ := a.RBAC.AdminRoles(req.Context(), uid)
		all, _ := a.RBAC.Roles(req.Context())
		names := map[string]string{}
		for _, r := range all {
			names[r.Code] = r.Name
		}
		roles := make([]map[string]string, 0, len(codes))
		for _, c := range codes {
			roles = append(roles, map[string]string{"code": c, "name": names[c]})
		}
		adminJSON(w, 0, "ok", map[string]any{
			"user_id": strconv.FormatInt(uid, 10), "nickname": brief.Nickname,
			"super": super, "roles": roles,
		})
	}))

	a.Mux.HandleFunc("GET /api/admin/v1/permissions", rbacAdmin(func(w http.ResponseWriter, req *http.Request) {
		adminJSON(w, 0, "ok", map[string]any{"items": rbac.Registry()})
	}))
	a.Mux.HandleFunc("GET /api/admin/v1/roles", rbacAdmin(func(w http.ResponseWriter, req *http.Request) {
		roles, err := a.RBAC.Roles(req.Context())
		if err != nil {
			adminJSON(w, 9, err.Error(), nil)
			return
		}
		adminJSON(w, 0, "ok", map[string]any{"items": roles})
	}))
	a.Mux.HandleFunc("GET /api/admin/v1/roles/{code}/permissions", rbacAdmin(func(w http.ResponseWriter, req *http.Request) {
		perms, err := a.RBAC.RolePerms(req.Context(), req.PathValue("code"))
		if err != nil {
			adminJSON(w, 9, err.Error(), nil)
			return
		}
		adminJSON(w, 0, "ok", map[string]any{"perms": perms})
	}))
	a.Mux.HandleFunc("PUT /api/admin/v1/roles/{code}/permissions", rbacWrite(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Perms []string `json:"perms"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			adminJSON(w, 1, "参数错误", nil)
			return
		}
		uid, super, perms := editorOf(req)
		code := req.PathValue("code")
		if err := a.RBAC.SetRolePerms(req.Context(), code, body.Perms, super, perms); err != nil {
			auditRBAC(req, uid, "ops:rbac:role:set", map[string]any{"code": code, "perms": body.Perms, "err": err.Error()}, audit.ResultDenied)
			adminJSON(w, 1, err.Error(), nil)
			return
		}
		auditRBAC(req, uid, "ops:rbac:role:set", map[string]any{"code": code, "perms": body.Perms}, audit.ResultOK)
		adminJSON(w, 0, "ok", map[string]any{"ok": true})
	}))
	a.Mux.HandleFunc("PUT /api/admin/v1/roles/{code}/scope", rbacWrite(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			RegionCodes    []string `json:"region_codes"`
			IncludeSubtree bool     `json:"include_subtree"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			adminJSON(w, 1, "参数错误", nil)
			return
		}
		uid, _, _ := editorOf(req)
		code := req.PathValue("code")
		if err := a.RBAC.SetRoleScope(req.Context(), code, body.RegionCodes, body.IncludeSubtree); err != nil {
			auditRBAC(req, uid, "ops:rbac:role:scope", map[string]any{"code": code, "err": err.Error()}, audit.ResultDenied)
			adminJSON(w, 1, err.Error(), nil)
			return
		}
		auditRBAC(req, uid, "ops:rbac:role:scope", map[string]any{"code": code, "region_codes": body.RegionCodes, "include_subtree": body.IncludeSubtree}, audit.ResultOK)
		adminJSON(w, 0, "ok", map[string]any{"ok": true})
	}))
	a.Mux.HandleFunc("POST /api/admin/v1/roles", rbacWrite(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Code string `json:"code"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			adminJSON(w, 1, "参数错误", nil)
			return
		}
		uid, _, _ := editorOf(req)
		if err := a.RBAC.CreateRole(req.Context(), body.Code, body.Name); err != nil {
			auditRBAC(req, uid, "ops:rbac:role:create", map[string]any{"code": body.Code, "err": err.Error()}, audit.ResultFailed)
			adminJSON(w, 1, err.Error(), nil)
			return
		}
		auditRBAC(req, uid, "ops:rbac:role:create", map[string]any{"code": body.Code, "name": body.Name}, audit.ResultOK)
		adminJSON(w, 0, "ok", map[string]any{"ok": true})
	}))
	a.Mux.HandleFunc("DELETE /api/admin/v1/roles/{code}", rbacWrite(func(w http.ResponseWriter, req *http.Request) {
		uid, _, _ := editorOf(req)
		code := req.PathValue("code")
		if err := a.RBAC.DeleteRole(req.Context(), code); err != nil {
			auditRBAC(req, uid, "ops:rbac:role:delete", map[string]any{"code": code, "err": err.Error()}, audit.ResultFailed)
			adminJSON(w, 1, err.Error(), nil)
			return
		}
		auditRBAC(req, uid, "ops:rbac:role:delete", map[string]any{"code": code}, audit.ResultOK)
		adminJSON(w, 0, "ok", map[string]any{"ok": true})
	}))
	a.Mux.HandleFunc("GET /api/admin/v1/admin-users/{id}/roles", rbacAdmin(func(w http.ResponseWriter, req *http.Request) {
		uid, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
		roles, err := a.RBAC.AdminRoles(req.Context(), uid)
		if err != nil {
			adminJSON(w, 9, err.Error(), nil)
			return
		}
		super, _ := a.RBAC.IsSuper(req.Context(), uid)
		adminJSON(w, 0, "ok", map[string]any{"roles": roles, "super": super})
	}))
	a.Mux.HandleFunc("PUT /api/admin/v1/admin-users/{id}/roles", rbacWrite(func(w http.ResponseWriter, req *http.Request) {
		uid, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
		var body struct {
			Roles []string `json:"roles"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			adminJSON(w, 1, "参数错误", nil)
			return
		}
		editor, super, perms := editorOf(req)
		if err := a.RBAC.AssignRoles(req.Context(), uid, body.Roles, super, perms); err != nil {
			auditRBAC(req, editor, "ops:rbac:assign", map[string]any{"user_id": uid, "roles": body.Roles, "err": err.Error()}, audit.ResultDenied)
			adminJSON(w, 1, err.Error(), nil)
			return
		}
		auditRBAC(req, editor, "ops:rbac:assign", map[string]any{"user_id": uid, "roles": body.Roles}, audit.ResultOK)
		adminJSON(w, 0, "ok", map[string]any{"ok": true})
	}))

	// 后台口令登录（生产主路径；手机号+密码，PBKDF2 哈希经 env 注入）
	adminLoginLimiter := newLoginLimiter()
	resetCodeLimiter := newSMSSendLimiter()
	a.Mux.HandleFunc("POST /api/admin/v1/auth/password", adminPasswordLogin(identityMod.Facade(), cfg, settingsReg, a.Audit, adminLoginLimiter))
	a.Mux.HandleFunc("POST /api/admin/v1/auth/password/reset/code", adminPasswordResetCode(identityMod.Facade(), cfg, a.Audit, resetCodeLimiter))
	a.Mux.HandleFunc("POST /api/admin/v1/auth/password/reset", adminPasswordReset(identityMod.Facade(), cfg, settingsReg, a.Audit, adminLoginLimiter))

	// 后台短信二次验证（p4 §3.4）：仅需已登录（令牌有效），不经后台准入/权限门禁
	adminTokenAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			token := adminToken(req)
			if token == "" {
				http.Error(w, `{"code":2,"message":"未登录"}`, http.StatusUnauthorized)
				return
			}
			uid, _, err := identityMod.Facade().Authenticate(req.Context(), token)
			if err != nil {
				http.Error(w, `{"code":2,"message":"登录已失效"}`, http.StatusUnauthorized)
				return
			}
			next(w, req.WithContext(context.WithValue(req.Context(), adminCtxKey{}, uid)))
		}
	}
	a.Mux.HandleFunc("POST /api/admin/v1/auth/password/change", adminTokenAuth(adminPasswordChange(identityMod.Facade(), cfg, settingsReg, a.Audit, adminLoginLimiter)))
	a.Mux.HandleFunc("POST /api/admin/v1/auth/step-up/code", adminTokenAuth(func(w http.ResponseWriter, req *http.Request) {
		uid, _ := adminUID(req)
		if err := identityMod.Facade().SendStepUpCode(req.Context(), uid); err != nil {
			adminJSON(w, 1, "验证码发送失败", nil)
			return
		}
		adminJSON(w, 0, "ok", map[string]any{"sent": true})
	}))
	a.Mux.HandleFunc("POST /api/admin/v1/auth/step-up", adminTokenAuth(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			adminJSON(w, 1, "参数错误", nil)
			return
		}
		uid, _ := adminUID(req)
		if err := identityMod.Facade().VerifyStepUpCode(req.Context(), uid, body.Code); err != nil {
			adminJSON(w, 1, "验证码错误", nil)
			return
		}
		adminJSON(w, 0, "ok", map[string]any{"ok": true})
	}))

	// 指标导出（Prometheus 文本格式）
	a.Mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(obs.Default().Render()))
	})

	// 健康检查
	a.Mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	a.Mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil || !a.ready.Load() {
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

// adminToken 后台令牌：优先 X-Admin-Token（便于 Nginx Basic Auth 占用 Authorization），
// 兼容标准 Authorization: Bearer（脚本/测试）。
func adminToken(req *http.Request) string {
	if t := strings.TrimSpace(req.Header.Get("X-Admin-Token")); t != "" {
		return t
	}
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// requireAdmin 后台路由守卫：先认证令牌，再校验后台准入（超管或任一 RBAC 角色），
// 再校验 IP 白名单与短信二次验证（登录失败达阈值，p4 §3.4）。未登录→401；不允许→403 并记 denied 审计。
func requireAdmin(id idcontract.Facade, rbacStore *rbac.Store, settingsReg *settings.Registry, rec *audit.Recorder, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := adminToken(req)
		if token == "" {
			http.Error(w, `{"code":2,"message":"未登录"}`, http.StatusUnauthorized)
			return
		}
		uid, _, err := id.Authenticate(req.Context(), token)
		if err != nil {
			http.Error(w, `{"code":2,"message":"登录已失效"}`, http.StatusUnauthorized)
			return
		}
		admin, err := rbacStore.IsAdmin(req.Context(), uid)
		if err != nil || !admin {
			recordAdminDenied(req, rec, uid, "admin:access:denied")
			http.Error(w, `{"code":3,"message":"无后台权限"}`, http.StatusForbidden)
			return
		}
		// IP 白名单（超管后台可配；空=不限制，支持前缀）
		if settingsReg != nil {
			if list, err := settingsReg.Get(req.Context(), 1, "admin.security.ip_allowlist"); err == nil && strings.TrimSpace(list) != "" {
				if !ipAllowed(clientIP(req), list) {
					recordAdminDenied(req, rec, uid, "admin:ip:denied")
					http.Error(w, `{"code":3,"message":"IP 不在允许范围"}`, http.StatusForbidden)
					return
				}
			}
			// 短信二次验证：登录失败达阈值（超管可配阈值/开关）
			if en, err := settingsReg.Get(req.Context(), 1, "admin.security.sms_2fa_enabled"); err == nil && strings.EqualFold(strings.TrimSpace(en), "true") {
				if need, _ := id.AdminStepUpRequired(req.Context(), uid); need {
					http.Error(w, `{"code":4,"message":"需短信二次验证"}`, http.StatusForbidden)
					return
				}
			}
		}
		next(w, req.WithContext(context.WithValue(req.Context(), adminCtxKey{}, uid)))
	}
}

// ipAllowed 白名单匹配：逗号分隔；支持精确与前缀（如 10.0.0.）。
func ipAllowed(ip, list string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false
	}
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if ip == item || strings.HasPrefix(ip, item) {
			return true
		}
	}
	return false
}

// adminCtxKey 后台已认证用户（requireAdmin 注入，requirePerm 读取）。
type adminCtxKey struct{}

func adminUID(req *http.Request) (int64, bool) {
	v, ok := req.Context().Value(adminCtxKey{}).(int64)
	return v, ok
}

func clientIP(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := req.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}

// requirePerm 在后台准入之上再校验权限点（服务端唯一权威，p4 §3.2/§3.3）。
// 需先经 requireAdmin（拿到已认证 uid）；非超管且缺权限→403 并记 denied。
func requirePerm(rbacStore *rbac.Store, rec *audit.Recorder, perm string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		uid, ok := adminUID(req)
		if !ok {
			http.Error(w, `{"code":2,"message":"未登录"}`, http.StatusUnauthorized)
			return
		}
		has, err := rbacStore.HasPerm(req.Context(), uid, perm)
		if err != nil || !has {
			recordAdminDenied(req, rec, uid, "admin:perm:denied")
			http.Error(w, `{"code":3,"message":"权限不足"}`, http.StatusForbidden)
			return
		}
		next(w, req)
	}
}

// recordAdminDenied 记后台拒绝审计（防爆破/越权尝试）。
func recordAdminDenied(req *http.Request, rec *audit.Recorder, uid int64, action string) {
	if rec == nil {
		return
	}
	_ = rec.Record(req.Context(), nil, audit.Entry{
		ActorID: uid, ActorType: audit.ActorAdmin, Channel: "admin", Action: action,
		Detail: map[string]any{"path": req.URL.Path, "method": req.Method}, Result: audit.ResultDenied,
		IP: clientIP(req), RequestID: obs.RequestID(req.Context()),
	})
}

// EnsureAdmins 按手机号授予平台管理员（运维引导：首个管理员/自助恢复）。
func EnsureAdmins(ctx context.Context, pool *db.Pool, blindKey []byte, phones []string) (int, error) {
	n := 0
	for _, p := range phones {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		hmac := sensitive.BlindIndex(blindKey, p)
		tag, err := pool.Exec(ctx, `
			UPDATE idn_user SET admin_role=1, authz_ver=authz_ver+1, updated_at=now()
			WHERE phone_hmac = $1`, hmac)
		if err != nil {
			return n, err
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
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

// SeedSensitive 幂等导入敏感词/引流词种子（seed 子命令）。
func SeedSensitive(ctx context.Context, a *App, path string) (int, error) {
	for _, m := range a.Modules {
		if tm, ok := m.(*trust.Module); ok {
			return tm.SeedWords(ctx, path)
		}
	}
	return 0, fmt.Errorf("trust 模块未装载")
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
	if err := migrate.NewRunner(pool).Apply(ctx, sources); err != nil {
		return err
	}
	if err := seedRBAC(ctx, pool); err != nil {
		return err
	}
	return seedAdminMenus(ctx, pool)
}

// seedRBAC 幂等写入后台五角色与默认权限矩阵（p4 §3，ADR-002）。
func seedRBAC(ctx context.Context, pool *db.Pool) error {
	return rbac.New(pool).Seed(ctx)
}

// Serve 启动 HTTP（api/admin 角色）。
func (a *App) Serve(ctx context.Context) error {
	a.ready.Store(true)
	srv := &http.Server{Addr: a.Config.HTTPAddr, Handler: obs.RequestIDMiddleware(a.Mux)}
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-ctx.Done()
		// 停机流程（p6 §3.2）：先置未就绪（负载探针据此摘流量）→ 等待排空窗口 → 优雅关闭（等在途请求）。
		a.ready.Store(false)
		drain := time.Duration(a.Config.DrainSeconds) * time.Second
		obs.Log(ctx).Info("开始排空：已置未就绪，等待摘流量", "drain_seconds", a.Config.DrainSeconds)
		if drain > 0 {
			// 注意：此处 ctx 已取消（正是触发停机的信号），排空窗口须独立计时
			time.Sleep(drain)
		}
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			obs.Default().Alert("shutdown_incomplete", err.Error())
		}
		obs.Log(ctx).Info("优雅停机完成")
	}()
	obs.Log(ctx).Info("catcube 启动", "addr", a.Config.HTTPAddr, "roles", a.Config.Roles)
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		<-drained // 等排空+优雅关闭完成，避免进程提前退出中断在途请求
		return nil
	}
	return err
}
