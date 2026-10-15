package redis

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer 最小 RESP 服务端：支持 PING/GET/SET/DEL/INCR/EXPIRE。
type fakeServer struct {
	mu    sync.Mutex
	data  map[string][]byte
	fail  bool
	lines chan string
}

func startFake(t *testing.T) (*fakeServer, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	s := &fakeServer{data: map[string][]byte{}}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s, ln.Addr().String()
}

func (s *fakeServer) serve(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	for {
		args, err := readCommand(br)
		if err != nil {
			return
		}
		s.mu.Lock()
		fail := s.fail
		s.mu.Unlock()
		switch strings.ToUpper(args[0]) {
		case "PING":
			if fail {
				_, _ = conn.Write([]byte("-ERR down\r\n"))
			} else {
				_, _ = conn.Write([]byte("+PONG\r\n"))
			}
		case "GET":
			s.mu.Lock()
			v, ok := s.data[args[1]]
			s.mu.Unlock()
			if fail {
				_, _ = conn.Write([]byte("-ERR down\r\n"))
			} else if !ok {
				_, _ = conn.Write([]byte("$-1\r\n"))
			} else {
				_, _ = conn.Write([]byte("$" + strconv.Itoa(len(v)) + "\r\n" + string(v) + "\r\n"))
			}
		case "SET":
			if fail {
				_, _ = conn.Write([]byte("-ERR down\r\n"))
				continue
			}
			s.mu.Lock()
			s.data[args[1]] = []byte(args[2])
			s.mu.Unlock()
			_, _ = conn.Write([]byte("+OK\r\n"))
		case "DEL":
			s.mu.Lock()
			delete(s.data, args[1])
			s.mu.Unlock()
			_, _ = conn.Write([]byte(":1\r\n"))
		case "INCR":
			s.mu.Lock()
			cur := 0
			if v, ok := s.data[args[1]]; ok {
				cur = atoi(string(v))
			}
			cur++
			s.data[args[1]] = []byte(strconv.Itoa(cur))
			s.mu.Unlock()
			_, _ = conn.Write([]byte(":" + strconv.Itoa(cur) + "\r\n"))
		case "EXPIRE":
			_, _ = conn.Write([]byte(":1\r\n"))
		default:
			_, _ = conn.Write([]byte("-ERR unknown\r\n"))
		}
	}
}

func readCommand(br *bufio.Reader) ([]string, error) {
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n := atoi(strings.TrimPrefix(strings.TrimSpace(line), "*"))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		h, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size := atoi(strings.TrimPrefix(strings.TrimSpace(h), "$"))
		buf := make([]byte, size+2)
		if _, err := br.Read(buf); err != nil {
			return nil, err
		}
		out = append(out, string(buf[:size]))
	}
	return out, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// 契约：真实 RESP 往返（假服务端）。
func TestClientRoundTrip(t *testing.T) {
	_, addr := startFake(t)
	c := New(addr, "")
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Fatal("不应存在")
	}
	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	v, ok, err := c.Get(ctx, "k")
	if err != nil || !ok || string(v) != "v" {
		t.Fatalf("get: %q %v %v", v, ok, err)
	}
	if n, err := c.Incr(ctx, "cnt"); err != nil || n != 1 {
		t.Fatalf("incr: %d %v", n, err)
	}
	if err := c.Del(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Fatal("应已删除")
	}
}

// 降级：主不可用 → 读写信道回退到备（进程内）。
func TestFallbackDegrade(t *testing.T) {
	s, addr := startFake(t)
	primary := New(addr, "")
	var hits int
	fb := NewFallback(primary, newMemCache(), func(error) { hits++ })
	ctx := context.Background()

	if err := fb.Set(ctx, "a", []byte("1"), time.Minute); err != nil {
		t.Fatal(err)
	}
	v, ok, err := fb.Get(ctx, "a")
	if err != nil || !ok || string(v) != "1" {
		t.Fatalf("正常态读取失败: %q %v %v", v, ok, err)
	}
	if hits != 0 {
		t.Fatalf("不应降级: %d", hits)
	}

	s.mu.Lock()
	s.fail = true
	s.mu.Unlock()
	if err := fb.Set(ctx, "b", []byte("2"), time.Minute); err != nil {
		t.Fatalf("降级写入应成功: %v", err)
	}
	v, ok, err = fb.Get(ctx, "b")
	if err != nil || !ok || string(v) != "2" {
		t.Fatalf("降级读取应命中备: %q %v %v", v, ok, err)
	}
	if hits < 2 {
		t.Fatalf("应触发降级回调（读+写）: %d", hits)
	}
}

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newMemCache() *memCache { return &memCache{m: map[string][]byte{}} }

func (c *memCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok, nil
}

func (c *memCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
	return nil
}

func (c *memCache) Del(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
	return nil
}

var _ = errors.Is
