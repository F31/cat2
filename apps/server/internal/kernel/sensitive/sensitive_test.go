package sensitive

import (
	"encoding/json"
	"testing"
)

func TestVisibleDefaultHidden(t *testing.T) {
	v := Wrap("13812345678")
	b, _ := json.Marshal(v)
	if string(b) != "null" {
		t.Fatalf("漏写策略的默认结果必须是「看不到」: %s", b)
	}
}

func TestVisibleReveal(t *testing.T) {
	b, _ := json.Marshal(Wrap("13812345678").Reveal(true))
	if string(b) != `"13812345678"` {
		t.Fatalf("放行后应输出明文: %s", b)
	}
	// 拒绝时仍不输出
	b2, _ := json.Marshal(Wrap("13812345678").Reveal(false))
	if string(b2) != "null" {
		t.Fatalf("拒绝时不得输出: %s", b2)
	}
}

func TestEncryptor(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("13812345678")
	c1, err := enc.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	c2, _ := enc.Encrypt(plain)
	if string(c1) == string(c2) {
		t.Fatal("随机 nonce：同明文密文必须不同")
	}
	back, err := enc.Decrypt(c1)
	if err != nil || string(back) != string(plain) {
		t.Fatalf("解密失败: %v", err)
	}
}

func TestBlindIndex(t *testing.T) {
	key := []byte("blind-key-32-bytes-00000000000000")
	a := BlindIndex(key, "13812345678")
	b := BlindIndex(key, "13812345678")
	c := BlindIndex(key, "13812345679")
	if string(a) != string(b) {
		t.Fatal("同值盲索引必须一致（支撑唯一校验）")
	}
	if string(a) == string(c) {
		t.Fatal("异值盲索引必须不同")
	}
}
