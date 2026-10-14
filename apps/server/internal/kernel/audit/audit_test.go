package audit

import (
	"context"
	"testing"
)

type fakeStore struct {
	entries []Entry
}

func (s *fakeStore) LastHash(context.Context) (string, error) {
	if len(s.entries) == 0 {
		return "", nil
	}
	return s.entries[len(s.entries)-1].Hash, nil
}

func (s *fakeStore) Append(_ context.Context, _ any, e Entry) error {
	s.entries = append(s.entries, e)
	return nil
}

func TestHashChain(t *testing.T) {
	store := &fakeStore{}
	rec := NewRecorder(store)
	ctx := context.Background()

	acts := []string{"trade:refund:execute", "trust:ban", "sys:settings:price"}
	for i, action := range acts {
		err := rec.Record(ctx, nil, Entry{
			ActorID: 1, ActorType: ActorAdmin, Channel: "admin",
			Action: action, Result: ResultOK, CreatedAt: int64(1000 + i),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(store.entries) != 3 {
		t.Fatal("条数不对")
	}
	// 链式校验
	prev := ""
	for _, e := range store.entries {
		if e.PrevHash != prev {
			t.Fatal("prev_hash 断链")
		}
		if e.Hash != Hash(prev, e) {
			t.Fatal("hash 校验失败")
		}
		prev = e.Hash
	}
}

func TestHashTamperDetect(t *testing.T) {
	e := Entry{ActorID: 1, Action: "trust:ban", Result: ResultOK, CreatedAt: 1000}
	h1 := Hash("", e)
	e.Detail = map[string]any{"before": "x"}
	if Hash("", e) == h1 {
		t.Fatal("篡改 detail 必须改变 hash")
	}
}
