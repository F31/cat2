// Package wechat 微信适配器骨架：登录（code2session）与支付。
// session_key 只在服务端短期使用，不下发、不入日志。
package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"catcube/internal/ports"
)

// AuthClient 微信登录。
type AuthClient struct {
	AppID     string
	AppSecret string
	HTTP      *http.Client
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

func (c *AuthClient) GetPhoneNumber(_ context.Context, sessionKey, encryptedData, iv string) (string, error) {
	// TODO: AES-128-CBC 解密 getPhoneNumber 数据（session_key 用完即弃）
	return "", fmt.Errorf("wechat: GetPhoneNumber 待接入")
}

var _ ports.WeChatAuth = (*AuthClient)(nil)

// PayClient 微信支付骨架（下单/回调验签/退款）。
type PayClient struct {
	MchID      string
	SerialNo   string
	PrivateKey string // PEM，启动配置注入
	APIv3Key   string
}

func NewPayClient(mchID, serialNo, privateKey, apiV3Key string) *PayClient {
	return &PayClient{MchID: mchID, SerialNo: serialNo, PrivateKey: privateKey, APIv3Key: apiV3Key}
}

func (p *PayClient) Prepay(_ context.Context, req ports.PayRequest) (string, error) {
	if p.MchID == "" {
		return "", fmt.Errorf("wechat pay: 未配置商户号")
	}
	return "", fmt.Errorf("wechat pay: Prepay 待接入（JSAPI 下单）")
}

func (p *PayClient) VerifyNotify(_ context.Context, _ map[string]string, _ []byte) (string, int64, error) {
	return "", 0, fmt.Errorf("wechat pay: VerifyNotify 待接入（平台证书验签）")
}

func (p *PayClient) Refund(_ context.Context, _, _ string, _ int64) error {
	return fmt.Errorf("wechat pay: Refund 待接入")
}

var _ ports.Pay = (*PayClient)(nil)
