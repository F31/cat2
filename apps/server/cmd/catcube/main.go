// catcube 唯一入口：serve | migrate | jobs | seed
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"catcube/internal/app"
	"catcube/internal/kernel/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: catcube <serve|migrate|jobs|seed> [flags]")
		os.Exit(1)
	}
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
	case "jobs":
		fmt.Println("jobs：单跑一次定时任务（待实现）")
	case "seed":
		dir := "../../contracts/seeds/dict"
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
		fmt.Printf("字典种子导入完成：%d 条（%s）\n", n, dir)
	default:
		fmt.Println("未知命令:", os.Args[1])
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
