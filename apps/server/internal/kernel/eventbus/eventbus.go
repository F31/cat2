// Package eventbus 领域事件总线：Outbox 同事务写入、至少一次投递、订阅方幂等、死信与重放。
// 事件即契约：载荷不含手机号、证件号等敏感字段。
package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"catcube/internal/kernel/obs"
)

// Event 领域事件。Topic 命名 `module.PastTense.vN`。
type Event struct {
	ID         string          `json:"id"`
	Topic      string          `json:"topic"`
	OccurredAt time.Time       `json:"occurred_at"`
	SpaceID    int16           `json:"space_id"`
	Payload    json.RawMessage `json:"payload"`
	// TraceID 触发链路的 trace（W3C trace-id，32 hex）：跨异步链路可追踪（p1 §5.6 前置）。
	TraceID string `json:"trace_id,omitempty"`
}

// Handler 订阅处理器；以事件 ID 幂等去重。
type Handler func(ctx context.Context, e Event) error

// Subscription 订阅注册项。
type Subscription struct {
	Topic   string
	Handler Handler
}

// OutboxStore 事件存储（Outbox）。业务表与事件在同一事务写入。
type OutboxStore interface {
	// Insert 在调用方事务内插入事件。
	Insert(ctx context.Context, tx any, e Event) error
	// Claim 取一批待投递事件（worker 角色）。
	Claim(ctx context.Context, limit int) ([]Event, error)
	// MarkDone / MarkRetry / MarkDead 更新投递状态。
	MarkDone(ctx context.Context, id string) error
	MarkRetry(ctx context.Context, id string) error
	MarkDead(ctx context.Context, id string) error
	// ListDead 死信列表（后台重放）。
	ListDead(ctx context.Context) ([]Event, error)
	Replay(ctx context.Context, id string) error
	// Stats 积压/最老事件年龄/死信数（监控）。
	Stats(ctx context.Context) (OutboxStats, error)
}

// OutboxStats Outbox 运行状态。
type OutboxStats struct {
	Backlog          int64
	Dead             int64
	OldestAgeSeconds int64
}

// AlertThresholds 监控告警阈值（0=不告警）。
type AlertThresholds struct {
	Backlog   int64
	OldestAge int64
	DeadCount int64
}

// Bus 进程内事件分发 + Outbox 持久化。
type Bus struct {
	store     OutboxStore
	mu        sync.RWMutex
	subs      map[string][]Handler
	processed ProcessedStore
}

// ProcessedStore 订阅方幂等（事件 ID 去重）。
type ProcessedStore interface {
	Seen(ctx context.Context, consumer, eventID string) (bool, error)
	Mark(ctx context.Context, consumer, eventID string) error
}

func NewBus(store OutboxStore, processed ProcessedStore) *Bus {
	return &Bus{store: store, subs: map[string][]Handler{}, processed: processed}
}

// Register 注册订阅。
func (b *Bus) Register(subs []Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range subs {
		b.subs[s.Topic] = append(b.subs[s.Topic], s.Handler)
	}
}

// Publish 在事务内写入 Outbox（投递由 worker 异步执行，至少一次）。
// 自动携带当前 trace（异步链路追踪）。
func (b *Bus) Publish(ctx context.Context, tx any, e Event) error {
	if e.TraceID == "" {
		e.TraceID = obs.TraceID(ctx)
	}
	if b.store == nil {
		return b.dispatch(ctx, e) // 无存储（测试）时直接分发
	}
	return b.store.Insert(ctx, tx, e)
}

func (b *Bus) dispatch(ctx context.Context, e Event) error {
	b.mu.RLock()
	handlers := append([]Handler(nil), b.subs[e.Topic]...)
	b.mu.RUnlock()
	for _, h := range handlers {
		if err := h(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// BatchStores 可选批量接口（适配器实现则启用批量确认/幂等，提升积压吞吐）。
type batchDoneStore interface {
	MarkDoneBatch(ctx context.Context, ids []string) error
}

type batchProcessedStore interface {
	SeenBatch(ctx context.Context, consumer string, ids []string) (map[string]bool, error)
	MarkBatch(ctx context.Context, consumer string, ids []string) error
}

// Relay worker 轮询 Outbox 投递。返回处理条数。
func (b *Bus) Relay(ctx context.Context, consumer string, limit int) (int, error) {
	if b.store == nil {
		return 0, nil
	}
	events, err := b.store.Claim(ctx, limit)
	if err != nil {
		return 0, err
	}
	// 批量幂等/确认（可用的适配器上启用，减少每事件一次往返）
	batchDone, _ := b.store.(batchDoneStore)
	batchProc, _ := b.processed.(batchProcessedStore)
	var seenSet map[string]bool
	if batchProc != nil {
		ids := make([]string, 0, len(events))
		for _, e := range events {
			ids = append(ids, e.ID)
		}
		seenSet, err = batchProc.SeenBatch(ctx, consumer, ids)
		if err != nil {
			return 0, err
		}
	}
	var doneIDs, markedIDs []string
	n := 0
	for _, e := range events {
		if b.processed != nil {
			seen := false
			if seenSet != nil {
				seen = seenSet[e.ID]
			} else {
				var err error
				seen, err = b.processed.Seen(ctx, consumer, e.ID)
				if err != nil {
					return n, err
				}
			}
			if seen {
				if batchDone != nil {
					doneIDs = append(doneIDs, e.ID)
				} else {
					_ = b.store.MarkDone(ctx, e.ID)
				}
				continue
			}
		}
		// 还原事件链路 trace，订阅方日志/下游事件可续接（逐事件，避免串链）
		evCtx := ctx
		if e.TraceID != "" {
			evCtx = obs.WithTrace(ctx, e.TraceID, obs.NewSpanID())
		}
		if err := b.dispatch(evCtx, e); err != nil {
			slog.Warn("事件投递失败（订阅处理器错误）", "topic", e.Topic, "event_id", e.ID, "err", err)
			obs.Default().Inc("catcube_outbox_retry_total")
			_ = b.store.MarkRetry(ctx, e.ID)
			continue
		}
		if b.processed != nil {
			if batchProc != nil {
				markedIDs = append(markedIDs, e.ID)
			} else if err := b.processed.Mark(ctx, consumer, e.ID); err != nil {
				return n, err
			}
		}
		if batchDone != nil {
			doneIDs = append(doneIDs, e.ID)
		} else if err := b.store.MarkDone(ctx, e.ID); err != nil {
			return n, err
		}
		n++
		obs.Default().Inc("catcube_outbox_relayed_total")
	}
	if batchProc != nil && len(markedIDs) > 0 {
		if err := batchProc.MarkBatch(ctx, consumer, markedIDs); err != nil {
			return n, err
		}
	}
	if batchDone != nil && len(doneIDs) > 0 {
		if err := batchDone.MarkDoneBatch(ctx, doneIDs); err != nil {
			return n, err
		}
	}
	return n, nil
}

// ReportStats 采集 Outbox 指标并按阈值告警（worker 定时调用）。
func (b *Bus) ReportStats(ctx context.Context, th AlertThresholds) (OutboxStats, error) {
	if b.store == nil {
		return OutboxStats{}, nil
	}
	st, err := b.store.Stats(ctx)
	if err != nil {
		return st, err
	}
	m := obs.Default()
	m.SetGauge("catcube_outbox_backlog", float64(st.Backlog))
	m.SetGauge("catcube_outbox_dead", float64(st.Dead))
	m.SetGauge("catcube_outbox_oldest_age_seconds", float64(st.OldestAgeSeconds))
	if th.Backlog > 0 && st.Backlog > th.Backlog {
		m.Alert("outbox_backlog_high", fmt.Sprintf("积压 %d 超过阈值 %d", st.Backlog, th.Backlog))
	}
	if th.OldestAge > 0 && st.OldestAgeSeconds > th.OldestAge {
		m.Alert("outbox_oldest_age_high", fmt.Sprintf("最老事件 %ds 超过阈值 %ds", st.OldestAgeSeconds, th.OldestAge))
	}
	if th.DeadCount > 0 && st.Dead > th.DeadCount {
		m.Alert("outbox_dead_high", fmt.Sprintf("死信 %d 超过阈值 %d", st.Dead, th.DeadCount))
	}
	return st, nil
}
