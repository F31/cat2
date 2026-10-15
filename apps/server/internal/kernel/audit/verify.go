package audit

import (
	"context"
	"fmt"
)

// ChainEntry 链上记录（供校验回放）。
type ChainEntry struct {
	ID       int64
	Entry    Entry
	PrevHash string
	Hash     string
}

// ChainLister 按链序（created_at,id）读取审计记录。
type ChainLister interface {
	ListChain(ctx context.Context, limit int) ([]ChainEntry, error)
}

// VerifyChain 校验哈希链连续性：返回首个断链位置（1 起；0 表示完整）与明细。
// 断链判定：本条 prev_hash 应等于上一条 hash；且按 canonical 重算 hash 应一致（防篡改）。
func VerifyChain(entries []ChainEntry) (int, string) {
	prev := ""
	for i, e := range entries {
		if e.PrevHash != prev {
			return i + 1, fmt.Sprintf("第 %d 条 prev_hash 与前一条 hash 不一致（链断裂）", i+1)
		}
		if got := Hash(prev, e.Entry); got != e.Hash {
			return i + 1, fmt.Sprintf("第 %d 条 hash 校验失败（记录被篡改）", i+1)
		}
		prev = e.Hash
	}
	return 0, ""
}

// VerifyWindow 校验窗口内链完整性：逐条重算 hash（用其自身 prev_hash，故不受截断影响）
// 并校验相邻 prev/hash 连续；首条不校验与前条连接（锚点）。
// 用于定时监控（近窗），避免历史遗留记录干扰；全量离线校验用 VerifyChain。
func VerifyWindow(entries []ChainEntry) (int, string) {
	if len(entries) == 0 {
		return 0, ""
	}
	for i, e := range entries {
		if got := Hash(e.PrevHash, e.Entry); got != e.Hash {
			return i + 1, fmt.Sprintf("第 %d 条 hash 校验失败（记录被篡改）", i+1)
		}
		if i > 0 && e.PrevHash != entries[i-1].Hash {
			return i + 1, fmt.Sprintf("第 %d 条 prev_hash 与前一条 hash 不一致（链断裂）", i+1)
		}
	}
	return 0, ""
}

// ChainListerWindow 按时间窗读取链记录。
type ChainListerWindow interface {
	ListChainFrom(ctx context.Context, sinceUnix int64, limit int) ([]ChainEntry, error)
}

// VerifyScanWindow 校验近窗链条（后台定时任务；tamper/断链触发告警）。
func VerifyScanWindow(ctx context.Context, l ChainListerWindow, sinceUnix int64, limit int) (int, string, error) {
	entries, err := l.ListChainFrom(ctx, sinceUnix, limit)
	if err != nil {
		return 0, "", err
	}
	idx, msg := VerifyWindow(entries)
	return idx, msg, nil
}

// VerifyForward 增量校验（自锚点起）：逐条重算 hash 并校验相邻连接；
// 首条需与锚点 hash 相接。返回首个问题位置（1 起；0=通过）。
func VerifyForward(entries []ChainEntry, anchorHash string) (int, string) {
	prev := anchorHash
	for i, e := range entries {
		if got := Hash(e.PrevHash, e.Entry); got != e.Hash {
			return i + 1, fmt.Sprintf("第 %d 条 hash 校验失败（记录被篡改）", i+1)
		}
		if e.PrevHash != prev {
			return i + 1, fmt.Sprintf("第 %d 条 prev_hash 与期望锚点不一致（链断裂/插入）", i+1)
		}
		prev = e.Hash
	}
	return 0, ""
}

// ChainListerAfter 增量读取（自游标 id 之后）。
type ChainListerAfter interface {
	ListChainAfter(ctx context.Context, afterID int64, limit int) ([]ChainEntry, error)
}

// VerifyIncremental 自游标起增量校验，返回问题位置与已校验到的 (id,hash)（供推进游标）。
func VerifyIncremental(ctx context.Context, l ChainListerAfter, afterID int64, anchorHash string, limit int) (int, string, int64, string, error) {
	entries, err := l.ListChainAfter(ctx, afterID, limit)
	if err != nil {
		return 0, "", afterID, anchorHash, err
	}
	idx, msg := VerifyForward(entries, anchorHash)
	if idx > 0 {
		return idx, msg, afterID, anchorHash, nil
	}
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		return 0, "", last.ID, last.Hash, nil
	}
	return 0, "", afterID, anchorHash, nil
}

// VerifyScan 拉取链并校验（后台定时任务/运维调用）。
func VerifyScan(ctx context.Context, l ChainLister, limit int) (int, string, error) {
	entries, err := l.ListChain(ctx, limit)
	if err != nil {
		return 0, "", err
	}
	idx, msg := VerifyChain(entries)
	return idx, msg, nil
}
