package app

import (
	"testing"
	"time"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	ts, err := NewTokenService()
	if err != nil {
		t.Fatal(err)
	}
	tok, err := ts.IssueAccess(1001, 2002, 3)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ts.Verify(tok)
	if err != nil {
		t.Fatalf("验签失败: %v", err)
	}
	if c.Sub != 1001 || c.Sid != 2002 || c.AuthzVer != 3 {
		t.Fatalf("claims 不符: %+v", c)
	}
	if c.Iss != "catcube" {
		t.Fatalf("issuer 不符: %q", c.Iss)
	}
}

func TestAccessTokenTampered(t *testing.T) {
	ts, err := NewTokenService()
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := ts.IssueAccess(1, 2, 1)
	// 篡改 payload（改角色/主体）
	if _, err := ts.Verify(tok[:len(tok)-2] + "xx"); err == nil {
		t.Fatal("篡改令牌应验签失败")
	}
	if _, err := ts.Verify("not-a-jwt"); err == nil {
		t.Fatal("非法格式应失败")
	}
}

func TestAccessTokenExpired(t *testing.T) {
	ts, err := NewTokenService()
	if err != nil {
		t.Fatal(err)
	}
	ts.accessTTL = -time.Minute // 签发即过期
	tok, _ := ts.IssueAccess(1, 2, 1)
	if _, err := ts.Verify(tok); err == nil {
		t.Fatal("过期令牌应失败")
	}
}

func TestNewRefreshTokenUnique(t *testing.T) {
	p1, h1, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	p2, h2, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatal("refresh token 不应重复")
	}
	if len(h1) != 32 || len(h2) != 32 {
		t.Fatalf("refresh 哈希长度应为 32: %d %d", len(h1), len(h2))
	}
	if string(h1) == string(h2) {
		t.Fatal("哈希不应相同")
	}
}
