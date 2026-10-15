// Package config 启动配置（三层配置的第一层：环境变量/文件）。
package config

import (
	"encoding/hex"
	"os"
	"strconv"
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

	// 监控告警阈值（p6 §4.6/§4.7；0=不告警）
	AlertOutboxBacklog   int64 // Outbox 积压条数阈值
	AlertOutboxOldestSec int64 // 最老事件年龄（秒）阈值
	AlertOutboxDead      int64 // 死信条数阈值
	AlertFulfillRetry    int   // 履约重试次数阈值
	ContactDailyLimit    int   // 单用户每日联系方式查看上限（0=不限，p6 §1.5）

	// Redis（S1：缓存；未配置则用进程内缓存）
	RedisAddr     string
	RedisPassword string

	// DevSMSCode 非空时作为固定短信验证码（开发/联调；生产必须留空）
	DevSMSCode string
}

// FromEnv 读取环境变量。
func FromEnv() Config {
	return Config{
		DatabaseURL:          getenv("CATCUBE_DATABASE_URL", "postgres://catcube:catcube@localhost:5432/catcube?sslmode=disable"),
		HTTPAddr:             getenv("CATCUBE_HTTP_ADDR", ":8080"),
		Roles:                split(getenv("CATCUBE_ROLES", "api,worker,scheduler,admin")),
		Modules:              split(getenv("CATCUBE_MODULES", "")),
		WeChatAppID:          getenv("CATCUBE_WECHAT_APPID", ""),
		WeChatAppSecret:      getenv("CATCUBE_WECHAT_SECRET", ""),
		IdentityEncKey:       getenv("CATCUBE_IDENTITY_ENC_KEY", ""),
		BlindKey:             getenv("CATCUBE_BLIND_KEY", ""),
		DevSMSCode:           getenv("CATCUBE_SMS_DEV_CODE", ""),
		Moderation:           getenv("CATCUBE_MODERATION", ""),
		AlertOutboxBacklog:   getenvInt("CATCUBE_ALERT_OUTBOX_BACKLOG", 1000),
		AlertOutboxOldestSec: getenvInt("CATCUBE_ALERT_OUTBOX_OLDEST_SECONDS", 300),
		AlertOutboxDead:      getenvInt("CATCUBE_ALERT_OUTBOX_DEAD", 100),
		AlertFulfillRetry:    int(getenvInt("CATCUBE_ALERT_FULFILL_RETRY", 5)),
		ContactDailyLimit:    int(getenvInt("CATCUBE_CONTACT_DAILY_LIMIT", 200)),
		RedisAddr:            getenv("CATCUBE_REDIS_ADDR", ""),
		RedisPassword:        getenv("CATCUBE_REDIS_PASSWORD", ""),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int64) int64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// BlindKeyBytes 解码盲索引密钥（缺失时用开发默认，与装配根一致）。
func (c Config) BlindKeyBytes() []byte {
	if len(c.BlindKey) == 64 {
		if b, err := hex.DecodeString(c.BlindKey); err == nil {
			return b
		}
	}
	return []byte("catcube-dev-blind-key-32bytes")
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
