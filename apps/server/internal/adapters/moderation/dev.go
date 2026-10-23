package moderation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"catcube/internal/ports"
)

// FailMarker 文本命中该标记即模拟机检失败（超时/不可用），用于降级演练与测试。
const FailMarker = "[FAULT]"

// DevClient 开发/演练态内容安全实现：默认放行；
// 含 FailMarker 的文本返回错误（等价于接口超时），驱动「审核中队列重试，不放行」（p6 §4.3）。
type DevClient struct{}

func NewDevClient() *DevClient { return &DevClient{} }

func (DevClient) CheckText(_ context.Context, req ports.ModerationRequest) (ports.ModerationResult, error) {
	if strings.Contains(req.Text, FailMarker) {
		return ports.ModerationResult{}, errors.New("moderation: 机检不可用（dev 模拟超时）")
	}
	return ports.ModerationResult{Pass: true}, nil
}

// CheckMedia 开发态异步占位：每次提交返回唯一 trace_id，测试通过回调接口驱动结论。
func (DevClient) CheckMedia(_ context.Context, req ports.ModerationRequest) (ports.ModerationResult, error) {
	if req.MediaURL == "" {
		return ports.ModerationResult{}, errors.New("moderation(dev): media_url 为空")
	}
	n := atomic.AddUint64(&devMediaSeq, 1)
	return ports.ModerationResult{Async: true, TraceID: fmt.Sprintf("dev-media-%d-%d", time.Now().UnixNano(), n)}, nil
}

var devMediaSeq uint64

var _ ports.Moderation = DevClient{}
