package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NotifyClient 微信订阅消息发送（小程序 subscribeMessage/send）。
// 模板 ID 由后台配置映射；未配置时调用方降级站内信。
type NotifyClient struct {
	AccessToken func(ctx context.Context) (string, error)
	HTTP        *http.Client
	BaseURL     string
}

func NewNotifyClient(tokenFn func(ctx context.Context) (string, error)) *NotifyClient {
	return &NotifyClient{AccessToken: tokenFn, HTTP: &http.Client{Timeout: 5 * time.Second}, BaseURL: "https://api.weixin.qq.com"}
}

// SendSubscribeMessage 发送订阅消息；data 以模板字段名（如 thing1）为 key。
func (c *NotifyClient) SendSubscribeMessage(ctx context.Context, openID, templateID string, data map[string]string) error {
	if strings.TrimSpace(openID) == "" || strings.TrimSpace(templateID) == "" {
		return fmt.Errorf("wechat notify: openid/template_id 为空")
	}
	token, err := c.AccessToken(ctx)
	if err != nil {
		return err
	}
	payload := make(map[string]map[string]string, len(data))
	for k, v := range data {
		payload[k] = map[string]string{"value": v}
	}
	body, _ := json.Marshal(map[string]any{
		"touser":      openID,
		"template_id": templateID,
		"data":        payload,
	})
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.weixin.qq.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/cgi-bin/message/subscribe/send?access_token=%s", baseURL, token), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var r struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if r.ErrCode != 0 {
		return fmt.Errorf("wechat notify: %d %s", r.ErrCode, r.ErrMsg)
	}
	return nil
}

// SendSMS 短信通道未实现（付费供应商接入后替换）。
func (c *NotifyClient) SendSMS(context.Context, string, string, map[string]string) error {
	return fmt.Errorf("wechat notify: 短信通道未配置")
}
