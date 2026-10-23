// Package wechat 微信适配器骨架：登录（code2session）与支付。
// session_key 只在服务端短期使用，不下发、不入日志。
package wechat

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"catcube/internal/ports"
)

// AuthClient 微信登录。
type AuthClient struct {
	AppID      string
	AppSecret  string
	EnvVersion string // 小程序码环境：release/trial/develop
	HTTP       *http.Client

	tk tokenCache
}

func NewAuthClient(appID, appSecret string) *AuthClient {
	return &AuthClient{AppID: appID, AppSecret: appSecret, HTTP: &http.Client{Timeout: 5 * time.Second}}
}

type code2sessionResp struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

func (c *AuthClient) Code2Session(ctx context.Context, code string) (string, string, error) {
	url := fmt.Sprintf("https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		c.AppID, c.AppSecret, code)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var r code2sessionResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", err
	}
	if r.ErrCode != 0 {
		return "", "", fmt.Errorf("wechat: code2session 失败 %d", r.ErrCode)
	}
	return r.OpenID, r.SessionKey, nil
}

// GetPhoneNumber 兼容旧版 getPhoneNumber：AES-128-CBC(session_key, iv) 解密并校验 watermark.appid。
// 新代码优先使用 GetPhoneNumberByCode；session_key 仅在本次解密内存中使用，不落日志。
// AccessToken 暴露 access_token（内容安全等微信服务复用；带进程内缓存）。
func (c *AuthClient) AccessToken(ctx context.Context) (string, error) {
	return c.accessToken(ctx)
}

func (c *AuthClient) GetPhoneNumber(_ context.Context, sessionKey, encryptedData, iv string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(sessionKey)
	if err != nil || len(key) != 16 {
		return "", errors.New("wechat: session_key 非法")
	}
	data, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil || len(data) == 0 {
		return "", errors.New("wechat: 手机号密文非法")
	}
	ivRaw, err := base64.StdEncoding.DecodeString(iv)
	if err != nil || len(ivRaw) != aes.BlockSize {
		return "", errors.New("wechat: 手机号 IV 非法")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	if len(data)%aes.BlockSize != 0 {
		return "", errors.New("wechat: 手机号密文长度非法")
	}
	plain := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, ivRaw).CryptBlocks(plain, data)
	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil {
		return "", err
	}
	var r struct {
		PhoneNumber     string `json:"phoneNumber"`
		PurePhoneNumber string `json:"purePhoneNumber"`
		Watermark       struct {
			AppID string `json:"appid"`
		} `json:"watermark"`
	}
	if err := json.Unmarshal(plain, &r); err != nil {
		return "", errors.New("wechat: 手机号明文非法")
	}
	if r.Watermark.AppID != c.AppID {
		return "", errors.New("wechat: 手机号 watermark.appid 不匹配")
	}
	phone := r.PurePhoneNumber
	if phone == "" {
		phone = r.PhoneNumber
	}
	if phone == "" {
		return "", errors.New("wechat: 手机号为空")
	}
	return phone, nil
}

func pkcs7Unpad(b []byte, blockSize int) ([]byte, error) {
	if len(b) == 0 || len(b)%blockSize != 0 {
		return nil, errors.New("wechat: 手机号填充非法")
	}
	n := int(b[len(b)-1])
	if n == 0 || n > blockSize || n > len(b) {
		return nil, errors.New("wechat: 手机号填充非法")
	}
	for _, v := range b[len(b)-n:] {
		if int(v) != n {
			return nil, errors.New("wechat: 手机号填充非法")
		}
	}
	return b[:len(b)-n], nil
}

var _ ports.WeChatAuth = (*AuthClient)(nil)
