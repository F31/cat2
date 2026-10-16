// Package pg PostgreSQL 适配器：queue（任务表）、search（pg_trgm）、
// eventbus Outbox 存储、settings/audit/quota/idempotency/migrate 的内核表实现。
package pg

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/ports"
)

// Queue PG 任务表队列。
type Queue struct{ pool *db.Pool }

func NewQueue(pool *db.Pool) *Queue { return &Queue{pool: pool} }

func (q *Queue) Enqueue(ctx context.Context, topic string, payload []byte) error {
	_, err := q.pool.Exec(ctx,
		`INSERT INTO sys_job_queue (topic, payload, status, created_at) VALUES ($1,$2,0,now())`,
		topic, payload)
	return err
}

func (q *Queue) Dequeue(ctx context.Context, topic string, limit int) ([][]byte, error) {
	rows, err := q.pool.Query(ctx,
		`UPDATE sys_job_queue SET status=1, locked_at=now()
		 WHERE id IN (SELECT id FROM sys_job_queue WHERE topic=$1 AND status=0 ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED)
		 RETURNING payload`, topic, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var p []byte
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

var _ ports.Queue = (*Queue)(nil)

// Search PG 检索（pg_trgm 相似度；扩展缺失/查询失败时降级为 ILIKE 简单检索，p6 §4.2）。
type Search struct {
	pool   *db.Pool
	simple bool // 强制走简单检索（扩展未装/降级演练）
}

func NewSearch(pool *db.Pool) *Search { return &Search{pool: pool} }

// NewSearchSimple 仅简单检索（无 pg_trgm 环境或降级态）。
func NewSearchSimple(pool *db.Pool) *Search { return &Search{pool: pool, simple: true} }

func (s *Search) Index(ctx context.Context, docType string, id string, doc map[string]any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	title, _ := doc["title"].(string)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO sys_search_doc (doc_type, doc_id, title, doc, updated_at) VALUES ($1,$2,$3,$4,now())
		 ON CONFLICT (doc_type, doc_id) DO UPDATE SET title=$3, doc=$4, updated_at=now()`,
		docType, id, title, raw)
	return err
}

func (s *Search) Delete(ctx context.Context, docType string, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sys_search_doc WHERE doc_type=$1 AND doc_id=$2`, docType, id)
	return err
}

func (s *Search) Query(ctx context.Context, docType string, q string, limit int) ([]string, error) {
	if s.simple {
		return s.querySimple(ctx, docType, q, limit)
	}
	out, err := s.queryTrigram(ctx, docType, q, limit)
	if err == nil {
		return out, nil
	}
	// 降级：pg_trgm 不可用（扩展缺失/超时）→ ILIKE 简单检索，业务可用性优先
	slog.Warn("检索降级为简单匹配", "err", err, "doc_type", docType)
	return s.querySimple(ctx, docType, q, limit)
}

func (s *Search) queryTrigram(ctx context.Context, docType, q string, limit int) ([]string, error) {
	return s.scanIDs(ctx,
		`SELECT doc_id FROM sys_search_doc WHERE doc_type=$1 AND title % $2
		 ORDER BY similarity(title, $2) DESC LIMIT $3`, docType, q, limit)
}

func (s *Search) querySimple(ctx context.Context, docType, q string, limit int) ([]string, error) {
	return s.scanIDs(ctx,
		`SELECT doc_id FROM sys_search_doc
		 WHERE doc_type=$1 AND (title ILIKE '%' || $2 || '%' OR doc::text ILIKE '%' || $2 || '%')
		 ORDER BY updated_at DESC LIMIT $3`, docType, q, limit)
}

func (s *Search) scanIDs(ctx context.Context, sql string, args ...any) ([]string, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

var _ ports.Search = (*Search)(nil)

// Outbox eventbus 存储。
type Outbox struct{ pool *db.Pool }

func NewOutbox(pool *db.Pool) *Outbox { return &Outbox{pool: pool} }

func (o *Outbox) Insert(ctx context.Context, tx any, e eventbus.Event) error {
	t, _ := tx.(db.Tx)
	exec := func(q string, args ...any) error {
		if t != nil {
			_, err := t.Exec(ctx, q, args...)
			return err
		}
		_, err := o.pool.Exec(ctx, q, args...)
		return err
	}
	return exec(
		`INSERT INTO sys_outbox (event_id, topic, payload, space_id, occurred_at, trace_id, status, retry_count)
		 VALUES ($1,$2,$3,$4,$5,$6,0,0)`,
		e.ID, e.Topic, e.Payload, e.SpaceID, e.OccurredAt, e.TraceID)
}

func (o *Outbox) Claim(ctx context.Context, limit int) ([]eventbus.Event, error) {
	rows, err := o.pool.Query(ctx,
		`UPDATE sys_outbox SET status=1
		 WHERE event_id IN (SELECT event_id FROM sys_outbox WHERE status=0 OR (status=2 AND retry_count<8)
		   ORDER BY occurred_at LIMIT $1 FOR UPDATE SKIP LOCKED)
		 RETURNING event_id, topic, payload, space_id, occurred_at, COALESCE(trace_id,'')`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []eventbus.Event
	for rows.Next() {
		var e eventbus.Event
		if err := rows.Scan(&e.ID, &e.Topic, &e.Payload, &e.SpaceID, &e.OccurredAt, &e.TraceID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// List 按发生顺序读取全部事件（只读，供投影回放；不改变投递状态）。
func (o *Outbox) List(ctx context.Context, limit int) ([]eventbus.Event, error) {
	rows, err := o.pool.Query(ctx,
		`SELECT event_id, topic, payload, space_id, occurred_at, COALESCE(trace_id,'') FROM sys_outbox
		 ORDER BY occurred_at, event_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []eventbus.Event
	for rows.Next() {
		var e eventbus.Event
		if err := rows.Scan(&e.ID, &e.Topic, &e.Payload, &e.SpaceID, &e.OccurredAt, &e.TraceID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Stats Outbox 积压/最老事件年龄/死信数（监控告警，p6 §4.6）。
func (o *Outbox) Stats(ctx context.Context) (eventbus.OutboxStats, error) {
	var st eventbus.OutboxStats
	var oldest *time.Time
	if err := o.pool.QueryRow(ctx, `
		SELECT
			count(1) FILTER (WHERE status IN (0,1,2)),
			count(1) FILTER (WHERE status = 9),
			min(occurred_at) FILTER (WHERE status IN (0,1,2))
		FROM sys_outbox`).Scan(&st.Backlog, &st.Dead, &oldest); err != nil {
		return st, err
	}
	if oldest != nil {
		st.OldestAgeSeconds = int64(time.Since(*oldest).Seconds())
	}
	return st, nil
}

func (o *Outbox) MarkDone(ctx context.Context, id string) error {
	_, err := o.pool.Exec(ctx, `UPDATE sys_outbox SET status=3, delivered_at=now() WHERE event_id=$1`, id)
	return err
}

// MarkDoneBatch 批量确认（投递通道吞吐优化：一条 UPDATE 代替 N 条）。
func (o *Outbox) MarkDoneBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := o.pool.Exec(ctx,
		`UPDATE sys_outbox SET status=3, delivered_at=now() WHERE event_id = ANY($1)`, ids)
	return err
}

func (o *Outbox) MarkRetry(ctx context.Context, id string) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE sys_outbox SET status=CASE WHEN retry_count+1>=8 THEN 9 ELSE 2 END, retry_count=retry_count+1 WHERE event_id=$1`, id)
	return err
}

func (o *Outbox) MarkDead(ctx context.Context, id string) error {
	_, err := o.pool.Exec(ctx, `UPDATE sys_outbox SET status=9 WHERE event_id=$1`, id)
	return err
}

func (o *Outbox) ListDead(ctx context.Context) ([]eventbus.Event, error) {
	rows, err := o.pool.Query(ctx,
		`SELECT event_id, topic, payload, space_id, occurred_at, COALESCE(trace_id,'') FROM sys_outbox WHERE status=9 ORDER BY occurred_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []eventbus.Event
	for rows.Next() {
		var e eventbus.Event
		if err := rows.Scan(&e.ID, &e.Topic, &e.Payload, &e.SpaceID, &e.OccurredAt, &e.TraceID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (o *Outbox) Replay(ctx context.Context, id string) error {
	_, err := o.pool.Exec(ctx, `UPDATE sys_outbox SET status=0, retry_count=0 WHERE event_id=$1 AND status=9`, id)
	return err
}

var _ eventbus.OutboxStore = (*Outbox)(nil)

// Processed 订阅方幂等表。
type Processed struct{ pool *db.Pool }

func NewProcessed(pool *db.Pool) *Processed { return &Processed{pool: pool} }

// SeenBatch 批量幂等查询：返回已处理的事件 ID 集合。
func (p *Processed) SeenBatch(ctx context.Context, consumer string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := p.pool.Query(ctx,
		`SELECT event_id FROM sys_event_processed WHERE consumer=$1 AND event_id = ANY($2)`, consumer, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// MarkBatch 批量登记已处理。
func (p *Processed) MarkBatch(ctx context.Context, consumer string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO sys_event_processed (consumer, event_id, processed_at)
		 SELECT $1, id, now() FROM unnest($2::text[]) AS id
		 ON CONFLICT DO NOTHING`, consumer, ids)
	return err
}

func (p *Processed) Seen(ctx context.Context, consumer, eventID string) (bool, error) {
	var n int
	err := p.pool.QueryRow(ctx,
		`SELECT count(1) FROM sys_event_processed WHERE consumer=$1 AND event_id=$2`, consumer, eventID).Scan(&n)
	return n > 0, err
}

func (p *Processed) Mark(ctx context.Context, consumer, eventID string) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO sys_event_processed (consumer, event_id, processed_at) VALUES ($1,$2,now())
		 ON CONFLICT DO NOTHING`, consumer, eventID)
	return err
}

var _ eventbus.ProcessedStore = (*Processed)(nil)

// KernelTables 内核表迁移（sys_ 前缀）。
func KernelTables() string {
	return `
CREATE TABLE IF NOT EXISTS sys_outbox (
	event_id text PRIMARY KEY,
	topic text NOT NULL,
	payload jsonb NOT NULL,
	space_id smallint NOT NULL DEFAULT 1,
	occurred_at timestamptz NOT NULL,
	status smallint NOT NULL DEFAULT 0, -- 0待投递 1投递中 2重试 3已投递 9死信
	retry_count int NOT NULL DEFAULT 0,
	delivered_at timestamptz,
	trace_id varchar(32) -- 触发链路 trace（W3C trace-id）
);
-- 兼容既有库：CREATE 之后补列（新库已含）
ALTER TABLE sys_outbox ADD COLUMN IF NOT EXISTS trace_id varchar(32);
CREATE INDEX IF NOT EXISTS idx_sys_outbox_status ON sys_outbox(status, occurred_at);

CREATE TABLE IF NOT EXISTS sys_event_processed (
	consumer text NOT NULL,
	event_id text NOT NULL,
	processed_at timestamptz NOT NULL,
	PRIMARY KEY (consumer, event_id)
);

CREATE TABLE IF NOT EXISTS sys_job_queue (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	topic text NOT NULL,
	payload jsonb NOT NULL,
	status smallint NOT NULL DEFAULT 0,
	locked_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sys_search_doc (
	doc_type text NOT NULL,
	doc_id text NOT NULL,
	title text NOT NULL,
	doc jsonb NOT NULL,
	updated_at timestamptz NOT NULL,
	PRIMARY KEY (doc_type, doc_id)
);

CREATE TABLE IF NOT EXISTS sys_settings (
	scope text NOT NULL,
	key text NOT NULL,
	value text NOT NULL,
	type text NOT NULL,
	version int NOT NULL DEFAULT 1,
	effective_at timestamptz,
	operator bigint,
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (scope, key)
);

CREATE TABLE IF NOT EXISTS sys_quota_counter (
	quota_key text NOT NULL,
	scope text NOT NULL,
	scope_id bigint NOT NULL,
	window_date date NOT NULL DEFAULT '1970-01-01',
	count bigint NOT NULL DEFAULT 0,
	PRIMARY KEY (quota_key, scope, scope_id, window_date)
);

CREATE TABLE IF NOT EXISTS sys_audit (
	id bigint PRIMARY KEY,
	actor_id bigint NOT NULL,
	actor_type smallint NOT NULL,
	channel varchar(16) NOT NULL,
	action varchar(64) NOT NULL,
	target_ref jsonb,
	detail jsonb,
	result smallint NOT NULL,
	ip inet,
	user_agent varchar(256),
	request_id varchar(64),
	prev_hash char(64) NOT NULL,
	hash char(64) NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);
-- 留存归档（p4 §5.7）：标记已归档行并记录边界哈希（链可离线续验）
ALTER TABLE sys_audit ADD COLUMN IF NOT EXISTS archived_at timestamptz;
CREATE INDEX IF NOT EXISTS idx_sys_audit_retention ON sys_audit(created_at) WHERE archived_at IS NULL;

CREATE TABLE IF NOT EXISTS sys_audit_archive (
	id bigint PRIMARY KEY,
	boundary_hash varchar(64) NOT NULL,
	last_audit_id bigint NOT NULL,
	rows_archived bigint NOT NULL,
	archived_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sys_idempotency (
	key text PRIMARY KEY,
	status smallint NOT NULL DEFAULT 0, -- 0处理中 1已完成
	resp_status int,
	resp_body bytea,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sys_feature_flag (
	key text PRIMARY KEY,
	enabled boolean NOT NULL DEFAULT false,
	rules jsonb,
	updated_at timestamptz NOT NULL DEFAULT now()
);
`
}
