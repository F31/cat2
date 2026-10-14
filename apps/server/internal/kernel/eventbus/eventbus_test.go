package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	events []Event
	done   map[string]bool
	retry  map[string]int
}

func newFakeStore() *fakeStore { return &fakeStore{done: map[string]bool{}, retry: map[string]int{}} }

func (s *fakeStore) Insert(_ context.Context, _ any, e Event) error {
	s.events = append(s.events, e)
	return nil
}

func (s *fakeStore) Claim(_ context.Context, limit int) ([]Event, error) {
	var out []Event
	for _, e := range s.events {
		if !s.done[e.ID] && s.retry[e.ID] < 8 {
			out = append(out, e)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *fakeStore) MarkDone(_ context.Context, id string) error  { s.done[id] = true; return nil }
func (s *fakeStore) MarkRetry(_ context.Context, id string) error { s.retry[id]++; return nil }
func (s *fakeStore) MarkDead(_ context.Context, id string) error  { s.retry[id] = 8; return nil }
func (s *fakeStore) ListDead(context.Context) ([]Event, error)    { return nil, nil }
func (s *fakeStore) Replay(_ context.Context, id string) error    { s.retry[id] = 0; return nil }

type fakeProcessed struct{ seen map[string]bool }

func (p *fakeProcessed) Seen(_ context.Context, consumer, eventID string) (bool, error) {
	return p.seen[consumer+eventID], nil
}
func (p *fakeProcessed) Mark(_ context.Context, consumer, eventID string) error {
	p.seen[consumer+eventID] = true
	return nil
}

func ev(id, topic string) Event {
	return Event{ID: id, Topic: topic, OccurredAt: time.Now(), Payload: json.RawMessage(`{}`)}
}

// 至少一次投递 + 订阅方幂等。
func TestPublishAndRelay(t *testing.T) {
	store := newFakeStore()
	proc := &fakeProcessed{seen: map[string]bool{}}
	bus := NewBus(store, proc)

	var got []string
	bus.Register([]Subscription{{Topic: "trade.OrderPaid.v1", Handler: func(_ context.Context, e Event) error {
		got = append(got, e.ID)
		return nil
	}}})

	if err := bus.Publish(context.Background(), nil, ev("e1", "trade.OrderPaid.v1")); err != nil {
		t.Fatal(err)
	}
	n, err := bus.Relay(context.Background(), "membership", 10)
	if err != nil || n != 1 || len(got) != 1 {
		t.Fatalf("n=%d got=%v err=%v", n, got, err)
	}
	// 重放：幂等去重，不再调用处理器
	n2, _ := bus.Relay(context.Background(), "membership", 10)
	if n2 != 0 || len(got) != 1 {
		t.Fatalf("幂等失败: n2=%d got=%v", n2, got)
	}
}

// 失败重试。
func TestRelayRetry(t *testing.T) {
	store := newFakeStore()
	bus := NewBus(store, nil)
	calls := 0
	bus.Register([]Subscription{{Topic: "t", Handler: func(context.Context, Event) error {
		calls++
		if calls < 2 {
			return errors.New("处理失败")
		}
		return nil
	}}})
	_ = bus.Publish(context.Background(), nil, ev("e1", "t"))
	if _, err := bus.Relay(context.Background(), "c", 10); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || store.retry["e1"] != 1 {
		t.Fatalf("首次失败应记重试: calls=%d retry=%d", calls, store.retry["e1"])
	}
	if _, err := bus.Relay(context.Background(), "c", 10); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !store.done["e1"] {
		t.Fatalf("二次应成功: calls=%d done=%v", calls, store.done)
	}
}
