package wechat

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// DevAuthClient 开发态假客户端：未配置 appid/secret 时启用，
// 使登录链路可在本地/CI 端到端跑通，绝不用于生产（装配根按配置选择）。
type DevAuthClient struct{}

func NewDevAuthClient() *DevAuthClient { return &DevAuthClient{} }

// 1x1 透明 PNG：开发态占位小程序码（绝不用于生产）。
const devCodePNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

// MiniProgramCode 开发态占位图（生产由真实 appid 生成）。
func (d *DevAuthClient) MiniProgramCode(_ context.Context, _, scene string) ([]byte, error) {
	if scene == "" {
		return nil, fmt.Errorf("wechat(dev): scene 为空")
	}
	return base64.StdEncoding.DecodeString(devCodePNG)
}

// Code2Session 用 code 派生稳定 openid（同一 code → 同一 openid）。
func (d *DevAuthClient) Code2Session(_ context.Context, code string) (string, string, error) {
	if code == "" {
		return "", "", fmt.Errorf("wechat(dev): code 为空")
	}
	sum := sha256.Sum256([]byte("dev-openid:" + code))
	return "dev_" + hex.EncodeToString(sum[:8]), "dev-session-key", nil
}

// GetPhoneNumber 旧签名（session_key 解密路径）：开发态不支持。
func (d *DevAuthClient) GetPhoneNumber(_ context.Context, _, _, _ string) (string, error) {
	return "", fmt.Errorf("wechat(dev): 请走 GetPhoneNumberByCode")
}

// GetPhoneNumberByCode 开发态：code 即手机号（形如 13800000001），
// 或以 phone:<手机号> 形式传入；便于用不同手机号造多个用户。
func (d *DevAuthClient) GetPhoneNumberByCode(_ context.Context, code string) (string, error) {
	if code == "" {
		return "", fmt.Errorf("wechat(dev): 手机号 code 为空")
	}
	if len(code) == 11 && code[0] == '1' {
		return code, nil
	}
	if len(code) > 6 && code[:6] == "phone:" {
		return code[6:], nil
	}
	// 其他输入：派生一个稳定的 11 位号码（避免链路中断）
	sum := sha256.Sum256([]byte("dev-phone:" + code))
	n := 0
	for _, b := range sum[:5] {
		n = n*256 + int(b)
	}
	return fmt.Sprintf("13%09d", n%1000000000), nil
}
