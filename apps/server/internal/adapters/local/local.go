// Package local 进程内适配器（S0 默认）。
package local

import (
	"context"
	"sync"
	"time"

	"catcube/internal/ports"
)

// Cache 进程内缓存（Redis 不可用时回退到此实现）。
type Cache struct {
	mu   sync.RWMutex
	data map[string]cacheItem
}

type cacheItem struct {
	val []byte
	exp time.Time
}

func NewCache() *Cache { return &Cache{data: map[string]cacheItem{}} }

func (c *Cache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	item, ok := c.data[key]
	c.mu.RUnlock()
	if !ok || (item.exp.Before(time.Now()) && !item.exp.IsZero()) {
		return nil, false, nil
	}
	return item.val, true, nil
}

func (c *Cache) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	c.data[key] = cacheItem{val: val, exp: exp}
	return nil
}

func (c *Cache) Del(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

var _ ports.Cache = (*Cache)(nil)

// RealtimeNoop 二期 WebSocket 前的空实现。
type RealtimeNoop struct{}

func (RealtimeNoop) Push(_ context.Context, _ int64, _ string, _ []byte) error { return nil }

var _ ports.Realtime = RealtimeNoop{}
