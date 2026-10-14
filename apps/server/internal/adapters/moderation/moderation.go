// Package moderation 微信内容安全适配器：文本同步、图片异步。
// 谁都不直接调微信内容安全接口，统一经本适配器；接口规格以微信官方最新文档为准。
package moderation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"catcube/internal/ports"
)

// Client 内容安全客户端。
type Client struct {
	AccessToken func(ctx context.Context) (string, error) // 由调用方提供 token 获取（带缓存）
	HTTP        *http.Client
}

func NewClient(tokenFn func(ctx context.Context) (string, error)) *Client {
	return &Client{AccessToken: tokenFn, HTTP: &http.Client{Timeout: 5 * time.Second}}
}

// CheckText 文本同步机检（msg_check）。
func (c *Client) CheckText(ctx context.Context, req ports.ModerationRequest) (ports.ModerationResult, error) {
	token, err := c.AccessToken(ctx)
	if err != nil {
		return ports.ModerationResult{}, err
	}
	body, _ := json.Marshal(map[string]any{"content": req.Text})
	url := fmt.Sprintf("https://api.weixin.qq.com/wxa/msg_sec_check?access_token=%s", token)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ports.ModerationResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return ports.ModerationResult{}, err // 超时/失败：调用方进「审核中」队列重试，不放行
	}
	defer resp.Body.Close()
	var r struct {
		ErrCode int `json:"errcode"`
		Detail  []struct {
			Label int `json:"label"`
		} `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return ports.ModerationResult{}, err
	}
	// errcode 0 通过；87014 命中
	return ports.ModerationResult{Pass: r.ErrCode == 0}, nil
}

// CheckMedia 图片异步机检（media_check_async）：回调前内容保持「审核中」。
func (c *Client) CheckMedia(_ context.Context, _ ports.ModerationRequest) (ports.ModerationResult, error) {
	// TODO: media_check_async 提交异步检测任务，结论经回调落 trs_moderation_record
	return ports.ModerationResult{Async: true}, nil
}

var _ ports.Moderation = (*Client)(nil)
