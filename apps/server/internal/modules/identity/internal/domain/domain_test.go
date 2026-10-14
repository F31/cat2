package domain

import (
	"testing"
	"time"
)

func TestLocked(t *testing.T) {
	now := time.Now()
	if Locked(4, now, now) {
		t.Fatal("未到阈值不应锁定")
	}
	if !Locked(5, now, now) {
		t.Fatal("达到阈值应锁定")
	}
	if Locked(5, now.Add(-FailWindow-time.Minute), now) {
		t.Fatal("窗口外的失败不应锁定")
	}
}

func TestSessionsToRevoke(t *testing.T) {
	mk := func(n int) []SessionBrief {
		out := make([]SessionBrief, n)
		for i := range out {
			out[i] = SessionBrief{ID: int64(i + 1), CreatedAt: time.Now().Add(time.Duration(i) * time.Minute)}
		}
		return out
	}
	if got := SessionsToRevoke(mk(2), MaxSessions); got != nil {
		t.Fatalf("未超上限不应吊销: %v", got)
	}
	got := SessionsToRevoke(mk(3), MaxSessions)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("应吊销最旧会话: %v", got)
	}
	got = SessionsToRevoke(mk(5), MaxSessions)
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("超限应吊销最旧的若干条: %v", got)
	}
}

func TestCanLoginAndPhone(t *testing.T) {
	if !CanLogin(StatusActive) || !CanLogin(StatusRestricted) {
		t.Fatal("正常/限制状态可登录")
	}
	if CanLogin(StatusBanned) || CanLogin(StatusDeactivated) {
		t.Fatal("封禁/注销不可登录")
	}
	if !ValidPhone("13800000001") || ValidPhone("2380000000") || ValidPhone("138000000") {
		t.Fatal("手机号格式判定错误")
	}
}
