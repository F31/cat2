// Package redis 最小 RESP 客户端 + 缓存适配器（S1 用；无外部依赖，仅标准库）。
// 仅实现本项目需要的命令：PING/GET/SET(PX)/DEL/INCR/EXPIRE。
// 连接不可用时由上层 Fallback 回退到进程内缓存（降级不中断业务）。
package redis

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"catcube/internal/ports"
)

// ErrClosed 连接关闭。
var ErrClosed = errors.New("redis: 连接不可用")

// Client 单连接（互斥复用，出错即重连）的 RESP 客户端。
type Client struct {
	addr     string
	password string
	dialTO   time.Duration

	mu   sync.Mutex
	conn net.Conn
	br   *bufio.Reader
}

// New 创建客户端（addr 形如 127.0.0.1:6379）。
func New(addr, password string) *Client {
	return &Client{addr: addr, password: password, dialTO: 3 * time.Second}
}

// Ping 连通性探测。
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, "PING")
	return err
}

// Get 读取（不存在返回 ok=false）。
func (c *Client) Get(ctx context.Context, key string) ([]byte, bool, error) {
	v, err := c.do(ctx, "GET", key)
	if err != nil {
		return nil, false, err
	}
	if v == nil {
		return nil, false, nil
	}
	b, _ := v.([]byte)
	return b, true, nil
}

// Set 写入，ttl>0 时带过期。
func (c *Client) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	args := []string{"SET", key, string(val)}
	if ttl > 0 {
		args = append(args, "PX", strconv.FormatInt(ttl.Milliseconds(), 10))
	}
	_, err := c.do(ctx, args...)
	return err
}

// Del 删除。
func (c *Client) Del(ctx context.Context, key string) error {
	_, err := c.do(ctx, "DEL", key)
	return err
}

// Incr 自增（限流计数）。
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	v, err := c.do(ctx, "INCR", key)
	if err != nil {
		return 0, err
	}
	n, _ := v.(int64)
	return n, nil
}

// Expire 设置过期（秒）。
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) error {
	_, err := c.do(ctx, "EXPIRE", key, strconv.FormatInt(int64(ttl.Seconds()), 10))
	return err
}

var _ ports.Cache = (*Client)(nil)

func (c *Client) do(ctx context.Context, args ...string) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensure(ctx); err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = c.conn.SetDeadline(dl)
	} else {
		_ = c.conn.SetDeadline(time.Now().Add(c.dialTO))
	}
	if err := writeCommand(c.conn, args); err != nil {
		c.reset()
		return nil, err
	}
	v, err := readReply(c.br)
	if err != nil {
		c.reset()
		return nil, err
	}
	return v, nil
}

func (c *Client) ensure(ctx context.Context) error {
	if c.conn != nil {
		return nil
	}
	d := net.Dialer{Timeout: c.dialTO}
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return err
	}
	c.conn = conn
	c.br = bufio.NewReader(conn)
	if c.password != "" {
		if err := writeCommand(conn, []string{"AUTH", c.password}); err != nil {
			c.reset()
			return err
		}
		if _, err := readReply(c.br); err != nil {
			c.reset()
			return err
		}
	}
	return nil
}

func (c *Client) reset() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn, c.br = nil, nil
}

// Close 关闭连接。
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.reset()
	return err
}

func writeCommand(w interface{ Write([]byte) (int, error) }, args []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	_, err := w.Write([]byte(b.String()))
	return err
}

func readReply(br *bufio.Reader) (any, error) {
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 {
		return nil, ErrClosed
	}
	body := strings.TrimSuffix(line[1:], "\r\n")
	switch line[0] {
	case '+':
		return body, nil
	case '-':
		return nil, fmt.Errorf("redis: %s", body)
	case ':':
		n, err := strconv.ParseInt(body, 10, 64)
		return n, err
	case '$':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := readFull(br, buf); err != nil {
			return nil, err
		}
		return buf[:n], nil
	case '*':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			v, err := readReply(br)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	return nil, fmt.Errorf("redis: 未知响应类型 %q", line[0])
}

func readFull(br *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := br.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
