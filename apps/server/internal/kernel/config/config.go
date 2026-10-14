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

// HasRole 是否启用某角色。
func (c Config) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}
