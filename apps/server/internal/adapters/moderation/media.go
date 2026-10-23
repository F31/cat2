package moderation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"catcube/internal/ports"
)

// CheckMedia 提交图片异步机检（media_check_async）。
// 微信仅接受公网 http(s) 地址；未配置对象存储/CDN 时明确失败，调用方保持「审核中」。
func (c *Client) CheckMedia(ctx context.Context, req ports.ModerationRequest) (ports.ModerationResult, error) {
	mediaURL := strings.TrimSpace(req.MediaURL)
	u, err := url.ParseRequestURI(mediaURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ports.ModerationResult{}, fmt.Errorf("moderation: media_url 必须是公网 http(s) 地址")
	}
	token, err := c.AccessToken(ctx)
	if err != nil {
		return ports.ModerationResult{}, err
	}
	body, _ := json.Marshal(map[string]any{
		"media_url":  mediaURL,
		"media_type": 2, // 2=图片
		"version":    2,
		"scene":      1,
	})
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.weixin.qq.com"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/wxa/media_check_async?access_token=%s", baseURL, token), bytes.NewReader(body))
	if err != nil {
		return ports.ModerationResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return ports.ModerationResult{}, err
	}
	defer resp.Body.Close()
	var r struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		TraceID string `json:"trace_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return ports.ModerationResult{}, err
	}
	if r.ErrCode != 0 {
		return ports.ModerationResult{}, fmt.Errorf("moderation: media_check_async %d %s", r.ErrCode, r.ErrMsg)
	}
	if r.TraceID == "" {
		return ports.ModerationResult{}, fmt.Errorf("moderation: media_check_async 未返回 trace_id")
	}
	return ports.ModerationResult{Async: true, TraceID: r.TraceID}, nil
}
