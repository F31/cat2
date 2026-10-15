//go:build integration

package pg

import (
	"context"
	"os"
	"testing"

	"catcube/internal/kernel/db"
	"catcube/internal/ports/suite"
)

func suitePool(t *testing.T) *db.Pool {
	t.Helper()
	url := os.Getenv("CATCUBE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 CATCUBE_TEST_DATABASE_URL")
	}
	pool, err := db.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	// 队列/检索依赖内核表
	if _, err := pool.Exec(context.Background(), KernelTables()); err != nil {
		t.Fatal(err)
	}
	return pool
}

// 端口契约：PG 队列与检索。
func TestQueueContract(t *testing.T)  { suite.Queue(t, NewQueue(suitePool(t))) }
func TestSearchContract(t *testing.T) { suite.Search(t, NewSearch(suitePool(t))) }

// 检索降级（p6 §4.2）：pg_trgm 不可用时回退 ILIKE 简单检索。
func TestSearchSimpleFallback(t *testing.T) {
	pool := suitePool(t)
	ctx := context.Background()
	s := NewSearchSimple(pool)
	if err := s.Index(ctx, "cube", "cube-fallback-1", map[string]any{"title": "精密钣金供应商"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Index(ctx, "cube", "cube-fallback-2", map[string]any{"title": "注塑模具"}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.Query(ctx, "cube", "钣金", 10)
	if err != nil {
		t.Fatalf("简单检索不应报错: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == "cube-fallback-1" {
			found = true
		}
		if id == "cube-fallback-2" {
			t.Fatalf("不应匹配无关文档: %v", ids)
		}
	}
	if !found {
		t.Fatalf("简单检索应命中子串: %v", ids)
	}
	_ = s.Delete(ctx, "cube", "cube-fallback-1")
	_ = s.Delete(ctx, "cube", "cube-fallback-2")
}
