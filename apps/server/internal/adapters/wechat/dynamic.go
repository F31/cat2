package wechat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	"catcube/internal/kernel/settings"
)

// DynamicClient 运行时读取后台「小程序设置」（appid/secret/env_version），
// 未配置时回退环境变量与开发态实现；密钥变更自动换新客户端（旧 token 缓存随之失效）。
type DynamicClient struct {
	reg            *settings.Registry
	fallbackAppID  string
	fallbackSecret string

	mu      sync.Mutex
	clients map[string]*AuthClient
	dev     *DevAuthClient
}

func NewDynamicClient(reg *settings.Registry, fallbackAppID, fallbackSecret string) *DynamicClient {
	return &DynamicClient{
		reg: reg, fallbackAppID: fallbackAppID, fallbackSecret: fallbackSecret,
		clients: map[string]*AuthClient{}, dev: NewDevAuthClient(),
	}
}

type wechatClient interface {
	Code2Session(ctx context.Context, code string) (string, string, error)
	GetPhoneNumber(ctx context.Context, sessionKey, encryptedData, iv string) (string, error)
	GetPhoneNumberByCode(ctx context.Context, code string) (string, error)
	MiniProgramCode(ctx context.Context, page, scene string) ([]byte, error)
}

func (d *DynamicClient) pick(ctx context.Context) wechatClient {
	appID, secret := d.fallbackAppID, d.fallbackSecret
	if d.reg != nil {
		if v, err := d.reg.Get(ctx, 1, "miniapp.appid"); err == nil && strings.TrimSpace(v) != "" {
			appID = strings.TrimSpace(v)
		}
		if v, err := d.reg.Get(ctx, 1, "miniapp.secret"); err == nil && strings.TrimSpace(v) != "" {
			secret = strings.TrimSpace(v)
		}
	}
	if appID == "" || secret == "" {
		return d.dev
	}
	sum := sha256.Sum256([]byte(secret))
	key := appID + "|" + hex.EncodeToString(sum[:])
	d.mu.Lock()
	defer d.mu.Unlock()
	if c, ok := d.clients[key]; ok {
		return c
	}
	c := NewAuthClient(appID, secret)
	if d.reg != nil {
		if v, err := d.reg.Get(ctx, 1, "miniapp.env_version"); err == nil && strings.TrimSpace(v) != "" {
			c.EnvVersion = strings.TrimSpace(v)
		}
	}
	// 仅保留当前 appid/secret 的客户端，避免密钥轮换后旧缓存常驻
	d.clients = map[string]*AuthClient{key: c}
	return c
}

func (d *DynamicClient) Code2Session(ctx context.Context, code string) (string, string, error) {
	return d.pick(ctx).Code2Session(ctx, code)
}

func (d *DynamicClient) GetPhoneNumber(ctx context.Context, sessionKey, encryptedData, iv string) (string, error) {
	return d.pick(ctx).GetPhoneNumber(ctx, sessionKey, encryptedData, iv)
}

func (d *DynamicClient) GetPhoneNumberByCode(ctx context.Context, code string) (string, error) {
	return d.pick(ctx).GetPhoneNumberByCode(ctx, code)
}

func (d *DynamicClient) MiniProgramCode(ctx context.Context, page, scene string) ([]byte, error) {
	return d.pick(ctx).MiniProgramCode(ctx, page, scene)
}
