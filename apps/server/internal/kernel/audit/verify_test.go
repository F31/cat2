package audit

import "testing"

func chain(n int) []ChainEntry {
	entries := make([]ChainEntry, 0, n)
	prev := ""
	for i := 0; i < n; i++ {
		e := Entry{ActorID: int64(i + 1), ActorType: ActorAdmin, Channel: "admin", Action: "x:y", Result: ResultOK, CreatedAt: int64(1000 + i)}
		h := Hash(prev, e)
		entries = append(entries, ChainEntry{ID: int64(i + 1), Entry: e, PrevHash: prev, Hash: h})
		prev = h
	}
	return entries
}

func TestVerifyChainOK(t *testing.T) {
	if idx, msg := VerifyChain(chain(5)); idx != 0 {
		t.Fatalf("完整链应通过: %d %s", idx, msg)
	}
}

func TestVerifyChainTamper(t *testing.T) {
	entries := chain(4)
	entries[2].Entry.Action = "篡改的动作" // 只改内容，不重算 hash
	if idx, _ := VerifyChain(entries); idx != 3 {
		t.Fatalf("篡改应定位第 3 条，实际 %d", idx)
	}
	entries = chain(4)
	entries[3].PrevHash = "deadbeef" // 断链
	if idx, _ := VerifyChain(entries); idx != 4 {
		t.Fatalf("断链应定位第 4 条，实际 %d", idx)
	}
}
