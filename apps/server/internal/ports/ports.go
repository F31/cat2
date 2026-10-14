// Package ports 能力端口（仅接口）。业务代码只依赖端口，不依赖适配器实现。
// 每个端口带契约测试套件（suite），同一套测试跑在每个适配器上。
package ports

import (
	"context"
	"io"
	"time"
)

// Cache 缓存端口（S0 进程内，S1 Redis）。
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// Queue 任务队列端口（一期 PG 任务表，二期可换 NATS/RabbitMQ）。
type Queue interface {
	Enqueue(ctx context.Context, topic string, payload []byte) error
	Dequeue(ctx context.Context, topic string, limit int) ([][]byte, error)
}

// Search 检索端口（一期 PG pg_trgm，可换 Meilisearch/ES）。
type Search interface {
	Index(ctx context.Context, docType string, id string, doc map[string]any) error
	Delete(ctx context.Context, docType string, id string) error
	Query(ctx context.Context, docType string, q string, limit int) ([]string, error)
}

// Storage 对象存储端口。
type Storage interface {
	// PresignUpload 生成直传预签名 URL。
	PresignUpload(ctx context.Context, bucket, objectKey, mime string, ttl time.Duration) (string, error)
	// URL 访问地址（可带规格，如缩略图）。
	URL(ctx context.Context, bucket, objectKey, spec string) (string, error)
	Put(ctx context.Context, bucket, objectKey string, r io.Reader) error
	Delete(ctx context.Context, bucket, objectKey string) error
}

// PayRequest 支付下单参数。
type PayRequest struct {
	OutTradeNo string
	AmountFen  int64
	Subject    string
	OpenID     string
}

// Pay 支付端口（微信）。
type Pay interface {
	Prepay(ctx context.Context, req PayRequest) (prepayID string, err error)
	// VerifyNotify 回调验签；返回 out_trade_no 与支付金额。
	VerifyNotify(ctx context.Context, headers map[string]string, body []byte) (outTradeNo string, amountFen int64, err error)
	Refund(ctx context.Context, outTradeNo, refundNo string, amountFen int64) error
}

// ModerationRequest 内容检测请求。
type ModerationRequest struct {
	SubjectType string // content / image / audio / video
	Text        string
	ImageIDs    []string
}

// ModerationResult 检测结果。
type ModerationResult struct {
	Pass   bool
	Labels []string
	Async  bool // 异步检测（回调前内容保持「审核中」）
}

// Moderation 内容安全端口（谁都不直接调微信接口）。
type Moderation interface {
	CheckText(ctx context.Context, req ModerationRequest) (ModerationResult, error)
	CheckMedia(ctx context.Context, req ModerationRequest) (ModerationResult, error)
}

// Notify 触达端口（订阅消息/短信）。
type Notify interface {
	SendSubscribeMessage(ctx context.Context, openID, templateID string, data map[string]string) error
	SendSMS(ctx context.Context, phone, templateID string, data map[string]string) error
}

// Realtime 实时推送端口（二期 WebSocket；一期 Noop）。
type Realtime interface {
	Push(ctx context.Context, userID int64, event string, payload []byte) error
}

// WeChatAuth 微信登录端口。
type WeChatAuth interface {
	// Code2Session code 换 openid 与 session_key（session_key 只在服务端短期使用，不下发不入日志）。
	Code2Session(ctx context.Context, code string) (openID, sessionKey string, err error)
	// GetPhoneNumber 解密手机号。
	GetPhoneNumber(ctx context.Context, sessionKey, encryptedData, iv string) (string, error)
}

// RealnameVerify 实名二要素核验端口（真实姓名 + 身份证号，接第三方接口；
// 不上传证件照；人脸核身高风险时触发，由实现方在内部编排）。
type RealnameVerify interface {
	// Verify 返回二要素是否一致。err 表示调用失败（区别于「不一致」）。
	Verify(ctx context.Context, name, idCardNo string) (matched bool, err error)
}

// LLM 大模型端口（p5 实现，接口先行）。
type LLM interface {
	Complete(ctx context.Context, prompt string, schema map[string]any) ([]byte, error)
	Embed(ctx context.Context, text string) ([]float32, error)
}
