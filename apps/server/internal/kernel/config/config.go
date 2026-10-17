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

	// 对象存储 / CDN（未配置时本地回退，开发态）
	OSSEndpoint string
	OSSBucket   string
	CDNHost     string
	OSSAK       string
	OSSSK       string

	// Moderation 内容安全实现：空=禁用（不机检）/dev=开发演练实现（含降级开关）
	Moderation string

	// DrainSeconds 停机排空窗口（先置未就绪让负载摘流量，再优雅关闭；p6 §3.2）
	DrainSeconds int64

	// 监控告警阈值（p6 §4.6/§4.7；0=不告警）
	AlertOutboxBacklog   int64 // Outbox 积压条数阈值
	AlertOutboxOldestSec int64 // 最老事件年龄（秒）阈值
	AlertOutboxDead      int64 // 死信条数阈值
	AlertFulfillRetry    int   // 履约重试次数阈值
	ContactDailyLimit    int   // 单用户每日联系方式查看上限（0=不限，p6 §1.5）
	AlertReviewBacklog   int   // 审核积压告警阈值（0=不告警，p6 §5.4）

	// Redis（S1：缓存；未配置则用进程内缓存）
	RedisAddr     string
	RedisPassword string

	// DevSMSCode 非空时作为固定短信验证码（开发/联调；生产必须留空）
	DevSMSCode string

	// OpenTelemetry（p1 §5.6 / p6 §5.1）：默认关闭；启用后经 OTLP gRPC 导出到 Collector
	OTelEnabled     bool
	OTelService     string
	OTelEndpoint    string
	OTelInsecure    bool
	OTelSampleRatio float64
}

// FromEnv 读取环境变量。
func FromEnv() Config {
	return Config{
		DatabaseURL:          getenv("CATCUBE_DATABASE_URL", "postgres://catcube:catcube@localhost:5432/catcube?sslmode=disable"),
		HTTPAddr:             getenv("CATCUBE_HTTP_ADDR", ":8080"),
		Roles:                split(getenv("CATCUBE_ROLES", "api,worker,scheduler,admin")),
		Modules:              split(getenv("CATCUBE_MODULES", "")),
		OSSEndpoint:          getenv("CATCUBE_OSS_ENDPOINT", ""),
		OSSBucket:            getenv("CATCUBE_OSS_BUCKET", ""),
		CDNHost:              getenv("CATCUBE_CDN_HOST", ""),
		OSSAK:                getenv("CATCUBE_OSS_AK", ""),
		OSSSK:                getenv("CATCUBE_OSS_SK", ""),
		WeChatAppID:          getenv("CATCUBE_WECHAT_APPID", ""),
		WeChatAppSecret:      getenv("CATCUBE_WECHAT_SECRET", ""),
		IdentityEncKey:       getenv("CATCUBE_IDENTITY_ENC_KEY", ""),
		BlindKey:             getenv("CATCUBE_BLIND_KEY", ""),
		DevSMSCode:           getenv("CATCUBE_SMS_DEV_CODE", ""),
		DrainSeconds:         getenvInt("CATCUBE_DRAIN_SECONDS", 5),
		Moderation:           getenv("CATCUBE_MODERATION", ""),
		AlertOutboxBacklog:   getenvInt("CATCUBE_ALERT_OUTBOX_BACKLOG", 1000),
		AlertOutboxOldestSec: getenvInt("CATCUBE_ALERT_OUTBOX_OLDEST_SECONDS", 300),
		AlertOutboxDead:      getenvInt("CATCUBE_ALERT_OUTBOX_DEAD", 100),
		AlertFulfillRetry:    int(getenvInt("CATCUBE_ALERT_FULFILL_RETRY", 5)),
		ContactDailyLimit:    int(getenvInt("CATCUBE_CONTACT_DAILY_LIMIT", 200)),
		AlertReviewBacklog:   int(getenvInt("CATCUBE_ALERT_REVIEW_BACKLOG", 200)),
		RedisAddr:            getenv("CATCUBE_REDIS_ADDR", ""),
		RedisPassword:        getenv("CATCUBE_REDIS_PASSWORD", ""),
		OTelEnabled:          getenvBool("CATCUBE_OTEL_ENABLED", false),
		OTelService:          getenv("CATCUBE_OTEL_SERVICE", "catcube"),
		OTelEndpoint:         getenv("CATCUBE_OTEL_ENDPOINT", "otel-collector:4317"),
		OTelInsecure:         getenvBool("CATCUBE_OTEL_INSECURE", true),
		OTelSampleRatio:      getenvFloat("CATCUBE_OTEL_SAMPLE_RATIO", 1),
	}
}

func getenvBool(k string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func getenvFloat(k string, def float64) float64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
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
