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
