package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// tokenCache access_token 缓存（微信 access_token 有效期 7200s，多实例需换集中缓存；一期单进程）。
type tokenCache struct {
	mu       sync.Mutex
	token    string
	expireAt time.Time
}

type tokenResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

// accessToken 获取（带缓存与提前 5 分钟刷新）。
func (c *AuthClient) accessToken(ctx context.Context) (string, error) {
	c.tk.mu.Lock()
	defer c.tk.mu.Unlock()
	if c.tk.token != "" && time.Now().Before(c.tk.expireAt) {
		return c.tk.token, nil
	}
	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s",
		c.AppID, c.AppSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.ErrCode != 0 || r.AccessToken == "" {
		return "", fmt.Errorf("wechat: 获取 access_token 失败 %d %s", r.ErrCode, r.ErrMsg)
	}
	c.tk.token = r.AccessToken
	c.tk.expireAt = time.Now().Add(time.Duration(r.ExpiresIn-300) * time.Second)
	return r.AccessToken, nil
}

// MiniProgramCode 生成小程序码（getwxacodeunlimit；返回 PNG 字节）。
func (c *AuthClient) MiniProgramCode(ctx context.Context, page, scene string) ([]byte, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	envVersion := c.EnvVersion
	if envVersion == "" {
		envVersion = "release"
	}
	body, _ := json.Marshal(map[string]any{
		"page": page, "scene": scene, "check_path": false, "env_version": envVersion,
		"width": 430,
	})
	url := "https://api.weixin.qq.com/wxa/getwxacodeunlimit?access_token=" + token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	// 微信错误以 JSON 返回；图片为 PNG（或 JPEG）。
	if len(raw) > 0 && raw[0] == '{' {
		var e struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
		}
		_ = json.Unmarshal(raw, &e)
		return nil, fmt.Errorf("wechat: 小程序码失败 %d %s", e.ErrCode, e.ErrMsg)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("wechat: 小程序码为空")
	}
	return raw, nil
}

type phoneResp struct {
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg"`
	PhoneInfo struct {
		PhoneNumber     string `json:"phoneNumber"`
		PurePhoneNumber string `json:"purePhoneNumber"`
		CountryCode     string `json:"countryCode"`
	} `json:"phone_info"`
}

// GetPhoneNumberByCode 手机号快速验证（新版 getuserphonenumber 接口，code 由前端 getPhoneNumber 事件返回）。
// 说明：ports.WeChatAuth.GetPhoneNumber 保持旧签名以兼容 session_key 解密路径；新链路走本方法。
func (c *AuthClient) GetPhoneNumberByCode(ctx context.Context, code string) (string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"code": code})
	url := "https://api.weixin.qq.com/wxa/business/getuserphonenumber?access_token=" + token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r phoneResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.ErrCode != 0 {
		return "", fmt.Errorf("wechat: 获取手机号失败 %d %s", r.ErrCode, r.ErrMsg)
	}
	phone := r.PhoneInfo.PurePhoneNumber
	if phone == "" {
		phone = r.PhoneInfo.PhoneNumber
	}
	if phone == "" {
		return "", fmt.Errorf("wechat: 手机号为空")
	}
	return phone, nil
}
