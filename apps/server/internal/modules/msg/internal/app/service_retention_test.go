package app

import (
	"context"
	"testing"
	"time"
)

// 聊天留存默认 180 天，配置可覆盖（p0 #2/D8）。
func TestRetentionCutoff(t *testing.T) {
	ctx := context.Background()

	// 默认 180 天
	s := New(Options{})
	def := s.retentionCutoff(ctx)
	if d := time.Since(def); d < 179*24*time.Hour || d > 181*24*time.Hour {
		t.Fatalf("默认留存应约 180 天，实际 %v", d)
	}

	// 配置为 30 天
	s2 := New(Options{RetentionDays: func(context.Context) int { return 30 }})
	d30 := s2.retentionCutoff(ctx)
	if d := time.Since(d30); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Fatalf("配置 30 天留存未生效，实际 %v", d)
	}

	// 非法值（<=0）回退默认 180
	s3 := New(Options{RetentionDays: func(context.Context) int { return 0 }})
	if d := time.Since(s3.retentionCutoff(ctx)); d < 179*24*time.Hour {
		t.Fatalf("非法配置应回退默认，实际 %v", d)
	}
}
