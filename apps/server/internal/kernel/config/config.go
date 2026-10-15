// Package config 启动配置（三层配置的第一层：环境变量/文件）。
package config

import (
	"os"
	"strings"
)

// Config 启动配置。
type Config struct {
	DatabaseURL string
	HTTPAddr    string
	Roles       []string // api / worker / scheduler / admin
	Modules     []string // modules.enabled；空=全部

	// 微信
	WeChatAppID     string
	WeChatAppSecret string

	// 敏感数据密钥（hex，64 位十六进制=32 字节）
	IdentityEncKey string // 字段加密（手机号/证件号）
	BlindKey       string // 盲索引（phone_hmac）独立密钥

	// Moderation 内容安全实现：空=禁用（不机检）/dev=开发演练实现（含降级开关）
	Moderation string

	// Redis（S1：缓存；未配置则用进程内缓存）
	RedisAddr     string
	RedisPassword string

	// DevSMSCode 非空时作为固定短信验证码（开发/联调；生产必须留空）
	DevSMSCode string
}

// FromEnv 读取环境变量。
func FromEnv() Config {
	return Config{
		DatabaseURL:     getenv("CATCUBE_DATABASE_URL", "postgres://catcube:catcube@localhost:5432/catcube?sslmode=disable"),
		HTTPAddr:        getenv("CATCUBE_HTTP_ADDR", ":8080"),
		Roles:           split(getenv("CATCUBE_ROLES", "api,worker,scheduler,admin")),
		Modules:         split(getenv("CATCUBE_MODULES", "")),
		WeChatAppID:     getenv("CATCUBE_WECHAT_APPID", ""),
		WeChatAppSecret: getenv("CATCUBE_WECHAT_SECRET", ""),
		IdentityEncKey:  getenv("CATCUBE_IDENTITY_ENC_KEY", ""),
		BlindKey:        getenv("CATCUBE_BLIND_KEY", ""),
		DevSMSCode:      getenv("CATCUBE_SMS_DEV_CODE", ""),
		Moderation:      getenv("CATCUBE_MODERATION", ""),
		RedisAddr:       getenv("CATCUBE_REDIS_ADDR", ""),
		RedisPassword:   getenv("CATCUBE_REDIS_PASSWORD", ""),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// HasModule 是否启用某模块（空列表=全部启用）。
func (c Config) HasModule(name string) bool {
	if len(c.Modules) == 0 {
		return true
	}
	for _, m := range c.Modules {
		if m == name {
			return true
		}
	}
	return false
}

// HasRole 是否启用某角色。
func (c Config) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}
