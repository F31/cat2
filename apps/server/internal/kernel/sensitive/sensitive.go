// Package sensitive 敏感字段保护：Visible[T] 默认不输出、AES-256-GCM 字段加密、
// HMAC 盲索引。漏写策略的结果是「看不到」，不是泄漏。
package sensitive

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

// Visible 敏感字段包装。只有显式调用 Reveal（经策略判定）后 MarshalJSON 才输出明文。
type Visible[T any] struct {
	value    T
	revealed bool
}

// Wrap 包装敏感值（默认不可见）。
func Wrap[T any](v T) Visible[T] { return Visible[T]{value: v} }

// Reveal 策略判定通过后显式放行。
func (v Visible[T]) Reveal(allow bool) Visible[T] {
	if allow {
		v.revealed = true
	}
	return v
}

// Value 服务侧内部取值（DTO 序列化不走这里）。
func (v Visible[T]) Value() T { return v.value }

// MarshalJSON 未放行时输出 null。
func (v Visible[T]) MarshalJSON() ([]byte, error) {
	if !v.revealed {
		return []byte("null"), nil
	}
	return json.Marshal(v.value)
}

// Encryptor 字段级 AES-256-GCM 加密器（信封加密时此为主密钥加密器）。
type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor key 必须 32 字节。
func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("sensitive: 密钥必须 32 字节")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Encryptor{gcm: gcm}, nil
}

// Encrypt 加密（随机 nonce 前置）。
func (e *Encryptor) Encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return e.gcm.Seal(nonce, nonce, plain, nil), nil
}

// Decrypt 解密。
func (e *Encryptor) Decrypt(data []byte) ([]byte, error) {
	ns := e.gcm.NonceSize()
	if len(data) < ns {
		return nil, errors.New("sensitive: 密文过短")
	}
	return e.gcm.Open(nil, data[:ns], data[ns:], nil)
}

// BlindIndex 盲索引（独立密钥 HMAC-SHA256），支撑「是否已注册」唯一校验。
func BlindIndex(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(value))
	return h.Sum(nil)
}
