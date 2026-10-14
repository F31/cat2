// Package app identity 令牌服务：Access 2h RS256 只含 sub/sid/authz_ver。
package app

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// TokenService RS256 JWT 签发与校验。
type TokenService struct {
	key      *rsa.PrivateKey
	issuer   string
	accessTTL time.Duration
}

// NewTokenService 生成开发用密钥对（生产经启动配置注入）。
func NewTokenService() (*TokenService, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &TokenService{key: key, issuer: "catcube", accessTTL: 2 * time.Hour}, nil
}

// Claims 只含 sub/sid/authz_ver（不含任何业务信息）。
type Claims struct {
	Sub      int64 `json:"sub"`
	Sid      int64 `json:"sid"`
	AuthzVer int   `json:"authz_ver"`
	Exp      int64 `json:"exp"`
	Iss      string `json:"iss"`
}

// IssueAccess 签发 Access Token。
func (s *TokenService) IssueAccess(userID, sessionID int64, authzVer int) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(Claims{
		Sub: userID, Sid: sessionID, AuthzVer: authzVer,
		Exp: time.Now().Add(s.accessTTL).Unix(), Iss: s.issuer,
	})
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(body))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return body + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Verify 校验签名与有效期，返回 Claims。
func (s *TokenService) Verify(token string) (Claims, error) {
	var c Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, errors.New("token: 格式错误")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return c, errors.New("token: 签名解码失败")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&s.key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		return c, errors.New("token: 签名无效")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(payload, &c); err != nil {
		return c, err
	}
	if time.Now().Unix() > c.Exp {
		return c, errors.New("token: 已过期")
	}
	return c, nil
}

// NewRefreshToken 生成 Refresh Token（存储只存哈希，7 天滚动轮换）。
func NewRefreshToken() (plain string, hash []byte, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(plain))
	return plain, sum[:], nil
}
