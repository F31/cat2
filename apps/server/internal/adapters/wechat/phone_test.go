package wechat

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func encryptPhone(t *testing.T, key, iv []byte, payload map[string]any) (string, string) {
	t.Helper()
	plain, _ := json.Marshal(payload)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	for i := 0; i < pad; i++ {
		plain = append(plain, byte(pad))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, plain)
	return base64.StdEncoding.EncodeToString(out), base64.StdEncoding.EncodeToString(iv)
}

func TestGetPhoneNumberLegacyDecrypt(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := []byte("abcdef0123456789")
	enc, ivB64 := encryptPhone(t, key, iv, map[string]any{
		"phoneNumber": "13800138000", "purePhoneNumber": "13800138000",
		"watermark": map[string]any{"appid": "wx-test", "timestamp": 1},
	})
	c := NewAuthClient("wx-test", "secret")
	phone, err := c.GetPhoneNumber(context.Background(), base64.StdEncoding.EncodeToString(key), enc, ivB64)
	if err != nil || phone != "13800138000" {
		t.Fatalf("解密结果错误: %q %v", phone, err)
	}
}

func TestGetPhoneNumberWatermarkMismatch(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := []byte("abcdef0123456789")
	enc, ivB64 := encryptPhone(t, key, iv, map[string]any{
		"purePhoneNumber": "13800138000",
		"watermark":       map[string]any{"appid": "wx-other"},
	})
	c := NewAuthClient("wx-test", "secret")
	if _, err := c.GetPhoneNumber(context.Background(), base64.StdEncoding.EncodeToString(key), enc, ivB64); err == nil {
		t.Fatal("watermark 不匹配应失败")
	}
}
