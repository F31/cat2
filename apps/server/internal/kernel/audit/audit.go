// Package audit 追加写审计 + 哈希链防篡改。
// 模型：谁(who)/在哪里(where)/什么时间(when)/做了什么(what)/结果(result)。
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ActorType 操作者类型。
type ActorType int16

const (
	ActorAdmin   ActorType = 1 // 后台账号
	ActorUser    ActorType = 2 // C 端用户
	ActorChannel ActorType = 3 // 运营商
	ActorSystem  ActorType = 4 // 系统任务
)

// Result 操作结果。
type Result int16

const (
	ResultOK     Result = 0 // 成功
	ResultDenied Result = 1 // 权限拒绝（安全审计最有价值的部分，必须记）
	ResultFailed Result = 2 // 失败
)

// Entry 审计记录（sys_audit）。
type Entry struct {
	ActorID   int64
	ActorType ActorType
	Channel   string // mp / admin / open / jobs
	Action    string // 权限点同名词，如 trade:refund:execute
	TargetRef string // EntityRef{type,id} JSON，系统级操作为空
	Detail    any    // 变更前后值快照（写入前已脱敏）
	Result    Result
	IP        string
	UserAgent string
	RequestID string
	PrevHash  string
	Hash      string
	CreatedAt int64
}

// Point 审计点声明（表驱动 + CI 校验）。
type Point struct {
	Action    string
	Required  bool // 必记 / 按需
	LogFailed bool // 记失败/拒绝
}

// Store 审计存储。
type Store interface {
	// LastHash 取链上最新 hash（空串表示链空）。
	LastHash(ctx context.Context) (string, error)
	// Append 追加写（与业务同事务由调用方保证）。
	Append(ctx context.Context, tx any, e Entry) error
}

// Hash 计算本条哈希：SHA256(prev_hash + canonical)。
func Hash(prev string, e Entry) string {
	c := canonical(e)
	sum := sha256.Sum256([]byte(prev + c))
	return hex.EncodeToString(sum[:])
}

// canonical 稳定序列化（map 键排序），保证链可离线校验。
func canonical(e Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%s|%s|%s|", e.ActorID, e.ActorType, e.Channel, e.Action, e.TargetRef)
	if e.Detail != nil {
		raw, _ := json.Marshal(sortedDetail(e.Detail))
		b.Write(raw)
	}
	fmt.Fprintf(&b, "|%d|%s|%s|%d", e.Result, e.IP, e.RequestID, e.CreatedAt)
	return b.String()
}

func sortedDetail(v any) any {
	if m, ok := v.(map[string]any); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(m))
		for _, k := range keys {
			out[k] = sortedDetail(m[k])
		}
		return out
	}
	if s, ok := v.([]any); ok {
		out := make([]any, len(s))
		for i := range s {
			out[i] = sortedDetail(s[i])
		}
		return out
	}
	return v
}

// Recorder 审计写入器。
type Recorder struct {
	store Store
}

func NewRecorder(store Store) *Recorder { return &Recorder{store: store} }

// Record 追加一条审计（自动补哈希链）。写失败不阻断业务由调用方决定。
func (r *Recorder) Record(ctx context.Context, tx any, e Entry) error {
	prev, err := r.store.LastHash(ctx)
	if err != nil {
		return err
	}
	e.PrevHash = prev
	e.Hash = Hash(prev, e)
	return r.store.Append(ctx, tx, e)
}
