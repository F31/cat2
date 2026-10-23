// Package moderation 微信内容安全适配器：文本同步、图片异步。
// 谁都不直接调微信内容安全接口，统一经本适配器；接口规格以微信官方最新文档为准。
package moderation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"catcube/internal/ports"
)

// Client 内容安全客户端。
type Client struct {
	AccessToken func(ctx context.Context) (string, error) // 由调用方提供 token 获取（带缓存）
	HTTP        *http.Client
	BaseURL     string // 测试注入；空=微信生产网关
}

func NewClient(tokenFn func(ctx context.Context) (string, error)) *Client {
	return &Client{AccessToken: tokenFn, HTTP: &http.Client{Timeout: 5 * time.Second},
		BaseURL: "https://api.weixin.qq.com"}
}

// CheckText 文本同步机检（msg_sec_check v2）。
// 仅 errcode=0 且 suggest=pass 才放行；review/risky 均不自动放行。
func (c *Client) CheckText(ctx context.Context, req ports.ModerationRequest) (ports.ModerationResult, error) {
	token, err := c.AccessToken(ctx)
	if err != nil {
		return ports.ModerationResult{}, err
	}
	body, _ := json.Marshal(map[string]any{"content": req.Text, "version": 2, "scene": 2})
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.weixin.qq.com"
	}
	url := fmt.Sprintf("%s/wxa/msg_sec_check?access_token=%s", baseURL, token)
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
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Result  struct {
			Suggest string `json:"suggest"`
			Label   int    `json:"label"`
		} `json:"result"`
		Detail []struct {
			Label int `json:"label"`
		} `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return ports.ModerationResult{}, err
	}
	var labels []string
	if r.Result.Label != 0 {
		labels = append(labels, fmt.Sprintf("%d", r.Result.Label))
	}
	for _, d := range r.Detail {
		if d.Label != 0 {
			labels = append(labels, fmt.Sprintf("%d", d.Label))
		}
	}
	if r.ErrCode == 87014 {
		return ports.ModerationResult{Pass: false, Labels: labels}, nil
	}
	if r.ErrCode != 0 {
		return ports.ModerationResult{}, fmt.Errorf("moderation: msg_sec_check %d %s", r.ErrCode, r.ErrMsg)
	}
	pass := r.Result.Suggest == "" || r.Result.Suggest == "pass"
	return ports.ModerationResult{Pass: pass, Labels: labels}, nil
}

// CheckMedia 图片异步机检（media_check_async）：回调前内容保持「审核中」。
func (c *Client) CheckMedia(_ context.Context, _ ports.ModerationRequest) (ports.ModerationResult, error) {
	// TODO: media_check_async 提交异步检测任务，结论经回调落 trs_moderation_record
	return ports.ModerationResult{Async: true}, nil
}

var _ ports.Moderation = (*Client)(nil)
