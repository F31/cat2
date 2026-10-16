// catcube 唯一入口：serve | migrate | jobs | seed
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"catcube/internal/adapters/otelx"
	"catcube/internal/app"
	"catcube/internal/kernel/config"
	"catcube/internal/kernel/obs"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: catcube <serve|migrate|jobs|seed> [flags]")
		os.Exit(1)
	}
	// 全局日志：JSON 结构化 + 统一脱敏（session_key/手机号/证件号不入日志，p6 §5.2）
	slog.SetDefault(slog.New(obs.NewRedactingHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))))
	cfg := config.FromEnv()
	// 命令行覆盖：--modules=a,b,c（未指定则用 CATCUBE_MODULES / 全部）
	for _, arg := range os.Args[2:] {
		if v, ok := strings.CutPrefix(arg, "--modules="); ok {
			if v == "" {
				cfg.Modules = nil
			} else {
				cfg.Modules = strings.Split(v, ",")
			}
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "serve":
		// OTel SDK（p1 §5.6）：默认关闭；启用后导出 Trace/Metric 到 Collector
		shutdownOTel, err := otelx.Setup(ctx, otelx.Config{
			Enabled: cfg.OTelEnabled, ServiceName: cfg.OTelService, Endpoint: cfg.OTelEndpoint,
			Insecure: cfg.OTelInsecure, SampleRatio: cfg.OTelSampleRatio,
		})
		if err != nil {
			fatal(err)
		}
		defer func() {
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdownOTel(sctx); err != nil {
				slog.Warn("otel 关闭异常", "err", err)
			}
		}()
		// OTel 日志桥接与指标镜像（p6 §5.1）：启用时在控制台 JSON 出口之外再扇出到 OTLP
		logHandler, shutdownLogs, err := otelx.SetupLogs(ctx, otelx.Config{
			Enabled: cfg.OTelEnabled, ServiceName: cfg.OTelService, Endpoint: cfg.OTelEndpoint,
			Insecure: cfg.OTelInsecure, SampleRatio: cfg.OTelSampleRatio,
		})
		if err != nil {
			fatal(err)
		}
		defer func() {
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdownLogs(sctx); err != nil {
				slog.Warn("otel 日志关闭异常", "err", err)
			}
		}()
		if logHandler != nil {
			base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
			slog.SetDefault(slog.New(obs.NewRedactingHandler(obs.NewTeeHandler(base, logHandler))))
			obs.RegisterOTelMetrics()
		}
		a, err := app.Wire(ctx, cfg)
		if err != nil {
			fatal(err)
		}
		if err := app.RunMigrations(ctx, a.Pool, a.Modules); err != nil {
			fatal(err)
		}
		// scheduler 角色：到期转态等定时任务
		if cfg.HasRole("scheduler") && a.Sched != nil {
			go a.Sched.Start(ctx)
		}
		if err := a.Serve(ctx); err != nil && err.Error() != "http: Server closed" {
			fatal(err)
		}
	case "migrate":
		a, err := app.Wire(ctx, cfg)
		if err != nil {
			fatal(err)
		}
		if err := app.RunMigrations(ctx, a.Pool, a.Modules); err != nil {
			fatal(err)
		}
		fmt.Println("迁移完成")
	case "probe":
		// 容器健康检查（distroless 无 shell）：探测 /readyz
		addr := cfg.HTTPAddr
		if strings.HasPrefix(addr, ":") {
			addr = "127.0.0.1" + addr
		}
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://" + addr + "/readyz")
		if err != nil {
			fmt.Fprintln(os.Stderr, "not ready:", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintln(os.Stderr, "not ready:", resp.StatusCode)
			os.Exit(1)
		}
	case "jobs":
		fmt.Println("jobs：单跑一次定时任务（待实现）")
	case "grant-admin":
		// 运维引导：将手机号设为平台管理员（用于首个后台账号/自助恢复）
		a, err := app.Wire(ctx, cfg)
		if err != nil {
			fatal(err)
		}
		if err := app.RunMigrations(ctx, a.Pool, a.Modules); err != nil {
			fatal(err)
		}
		phones := os.Args[2:]
		n, err := app.EnsureAdmins(ctx, a.Pool, cfg.BlindKeyBytes(), phones)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("已授予 %d 个账号平台管理员（目标 %d 个手机号）\n", n, len(phones))
	case "seed":
		dir := "../../contracts/seeds/dict"
		wordsPath := "../../contracts/seeds/sensitive/words.json"
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		a, err := app.Wire(ctx, cfg)
		if err != nil {
			fatal(err)
		}
		if err := app.RunMigrations(ctx, a.Pool, a.Modules); err != nil {
			fatal(err)
		}
		n, err := app.SeedDict(ctx, a, dir)
		if err != nil {
			fatal(err)
		}
		wn, err := app.SeedSensitive(ctx, a, wordsPath)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("种子导入完成：字典 %d 条（%s）、敏感词/引流词 %d 条（%s）\n", n, dir, wn, wordsPath)
	default:
		fmt.Println("未知命令:", os.Args[1])
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
