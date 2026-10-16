// loadtest 轻量压测器（p6 §2.2/§2.3）：无外部依赖，对运行中的 S0/S1 实例跑场景，
// 输出 P50/P95/P99、RPS、错误率，并按门槛判定退出码（供 CI/上线前置使用）。
//
// 用法：
//
//	go run . -base http://127.0.0.1:8080 -token <access_token> -c 50 -d 30s
//
// 场景可用 -scenarios 选择（逗号分隔）：feed,search,detail,orders
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Threshold 场景门槛（P95/P99 毫秒）。
type Threshold struct {
	P95ms int
	P99ms int
}

type scenario struct {
	name   string
	method string
	path   string
	body   any
	th     Threshold
}

func main() {
	base := flag.String("base", "http://127.0.0.1:8080", "目标地址")
	token := flag.String("token", "", "Access Token（部分场景需要）")
	ref := flag.String("ref", "", "详情场景目标 EntityRef，如 user:1 / content.project:1")
	concurrency := flag.Int("c", 20, "并发数")
	duration := flag.Duration("d", 15*time.Second, "压测时长")
	scenariosFlag := flag.String("scenarios", "feed,search,detail", "场景：feed,search,detail,orders")
	warmup := flag.Duration("warmup", 2*time.Second, "预热时长（不计入统计）")
	flag.Parse()

	all := map[string]scenario{
		"feed":   {name: "feed", method: http.MethodGet, path: "/api/mp/v1/feed", th: Threshold{P95ms: 3000}},
		"search": {name: "search", method: http.MethodGet, path: "/api/mp/v1/search?type=content.project&q=项目", th: Threshold{P99ms: 1000}},
		"detail": {name: "detail", method: http.MethodGet, path: "/api/mp/v1/aggregate/detail?ref=" + *ref, th: Threshold{P99ms: 1000}},
		"orders": {name: "orders", method: http.MethodPost, path: "/api/mp/v1/orders",
			body: map[string]any{"biz_type": "member", "amount_fen": 9900,
				"price_snapshot": map[string]any{"price_fen": 9900, "years": 1}},
			th: Threshold{P99ms: 1000}},
	}
	var picked []scenario
	for _, name := range strings.Split(*scenariosFlag, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		s, ok := all[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "未知场景 %q\n", name)
			os.Exit(2)
		}
		picked = append(picked, s)
	}
	if len(picked) == 0 {
		fmt.Fprintln(os.Stderr, "至少选择一个场景")
		os.Exit(2)
	}

	// 连接池随并发放大：默认 MaxIdleConnsPerHost=2 会造成高 RPS 下连接频繁重建与瞬时错误
	transport := &http.Transport{
		MaxIdleConns:        *concurrency * 4,
		MaxIdleConnsPerHost: *concurrency * 2,
		MaxConnsPerHost:     *concurrency * 2,
		IdleConnTimeout:     30 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: transport}
	failed := false
	fmt.Printf("目标 %s 并发 %d 时长 %s\n", *base, *concurrency, *duration)
	for _, s := range picked {
		res := run(client, *base, *token, s, *concurrency, *warmup, *duration)
		fmt.Printf("\n场景 %-7s 请求 %6d  错误率 %5.2f%%  RPS %8.1f  P50 %5dms  P95 %6dms  P99 %6dms  Max %6dms\n",
			s.name, res.total, res.errRate(), res.rps, res.p50, res.p95, res.p99, res.max)
		verdict := "PASS"
		if s.th.P95ms > 0 && int(res.p95) > s.th.P95ms {
			verdict = fmt.Sprintf("FAIL（P95 %dms > 门槛 %dms）", int(res.p95), s.th.P95ms)
			failed = true
		}
		if s.th.P99ms > 0 && int(res.p99) > s.th.P99ms {
			verdict = fmt.Sprintf("FAIL（P99 %dms > 门槛 %dms）", int(res.p99), s.th.P99ms)
			failed = true
		}
		if res.errRate() >= 1.0 {
			verdict = fmt.Sprintf("FAIL（错误率 %.2f%% ≥ 1%%）", res.errRate())
			failed = true
		}
		if len(res.errs) > 0 {
			parts := make([]string, 0, len(res.errs))
			for msg, n := range res.errs {
				parts = append(parts, fmt.Sprintf("%s×%d", msg, n))
			}
			sort.Strings(parts)
			fmt.Printf("\n       错误构成：%s", strings.Join(parts, " "))
		}
		fmt.Printf("\n       判定：%s", verdict)
		if len(res.codes) > 0 {
			parts := make([]string, 0, len(res.codes))
			for code, n := range res.codes {
				parts = append(parts, fmt.Sprintf("%d×%d", code, n))
			}
			sort.Strings(parts)
			fmt.Printf("  状态码：%s", strings.Join(parts, " "))
		}
		fmt.Println()
	}
	if failed {
		os.Exit(1)
	}
}

type result struct {
	total              int64
	errors             int64
	p50, p95, p99, max int64
	rps                float64
	mu                 sync.Mutex
	lat                []int64
	codes              map[int]int64
	errs               map[string]int64
}

func (r *result) errRate() float64 {
	if r.total == 0 {
		return 0
	}
	return float64(r.errors) * 100 / float64(r.total)
}

func run(client *http.Client, base, token string, s scenario, concurrency int, warmup, duration time.Duration) *result {
	var res result
	ctx, cancel := context.WithTimeout(context.Background(), warmup+duration)
	defer cancel()

	do := func(countLatency bool) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if ctx.Err() != nil {
				return // 运行窗口结束导致的取消不计为错误
			}
			start := time.Now()
			status, err := call(ctx, client, base, token, s)
			el := time.Since(start).Milliseconds()
			if ctx.Err() != nil {
				return
			}
			if err != nil || status < 200 || status >= 300 {
				if countLatency {
					atomic.AddInt64(&res.errors, 1)
					res.mu.Lock()
					if res.errs == nil {
						res.errs = map[string]int64{}
					}
					if err != nil {
						res.errs[errText(err)]++
					} else {
						res.errs[fmt.Sprintf("HTTP %d", status)]++
					}
					res.mu.Unlock()
				}
			}
			if countLatency {
				res.mu.Lock()
				if res.codes == nil {
					res.codes = map[int]int64{}
				}
				res.codes[status]++
				res.mu.Unlock()
				atomic.AddInt64(&res.total, 1)
				res.mu.Lock()
				res.lat = append(res.lat, el)
				res.mu.Unlock()
			}
		}
	}

	// 预热
	warmCtx, warmCancel := context.WithTimeout(ctx, warmup)
	_ = warmCtx
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); do(false) }()
	}
	time.Sleep(warmup)
	warmCancel()

	// 正式计量
	start := time.Now()
	var wg2 sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			for {
				select {
				case <-stop:
					return
				default:
					do(true)
				}
			}
		}()
	}
	time.Sleep(duration)
	close(stop)
	wg.Wait()
	wg2.Wait()
	elapsed := time.Since(start).Seconds()

	res.mu.Lock()
	sort.Slice(res.lat, func(i, j int) bool { return res.lat[i] < res.lat[j] })
	if n := len(res.lat); n > 0 {
		res.p50 = percentile(res.lat, 50)
		res.p95 = percentile(res.lat, 95)
		res.p99 = percentile(res.lat, 99)
		res.max = res.lat[n-1]
	}
	res.mu.Unlock()
	if elapsed > 0 {
		res.rps = float64(res.total) / elapsed
	}
	return &res
}

func errText(err error) string {
	msg := err.Error()
	if len(msg) > 60 {
		msg = msg[:60]
	}
	return msg
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func call(ctx context.Context, client *http.Client, base, token string, s scenario) (int, error) {
	var body io.Reader
	if s.body != nil {
		raw, _ := json.Marshal(s.body)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, s.method, base+s.path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// 下单类接口需要幂等键，避免压测把幂等命中当异常
	if s.method == http.MethodPost {
		req.Header.Set("Idempotency-Key", fmt.Sprintf("load-%d", time.Now().UnixNano()))
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
