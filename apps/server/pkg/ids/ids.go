// Package ids 生成 int64 时间有序 ID（应用侧生成），JSON 一律输出为字符串，
// 避免 JS 53 位精度丢失。
//
// 布局：41 bit 毫秒时间戳（相对自定义纪元）| 12 bit 序列 | 10 bit 节点。
package ids

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"
)

const (
	epochMillis   = 1735689600000 // 2025-01-01T00:00:00Z
	seqBits       = 12
	nodeBits      = 10
	maxSeq        = 1<<seqBits - 1
	maxNode       = 1<<nodeBits - 1
	tsShift       = seqBits + nodeBits
)

// ID 时间有序 int64 主键。
type ID int64

var (
	mu       sync.Mutex
	node     int64 = 1
	lastMillis int64
	seq      int64
)

// SetNode 设置节点号（0~1023），多实例部署时按实例分配。
func SetNode(n int64) { node = n & maxNode }

// New 生成一个时间有序 ID。
func New() ID {
	mu.Lock()
	defer mu.Unlock()
	now := time.Now().UnixMilli() - epochMillis
	if now == lastMillis {
		seq = (seq + 1) & maxSeq
		if seq == 0 { // 序列耗尽，自旋到下一毫秒
			for now <= lastMillis {
				now = time.Now().UnixMilli() - epochMillis
			}
		}
	} else {
		seq = 0
	}
	lastMillis = now
	return ID(now<<tsShift | node<<seqBits | seq)
}

func (i ID) String() string { return strconv.FormatInt(int64(i), 10) }

// Time 还原 ID 的生成时间。
func (i ID) Time() time.Time {
	return time.UnixMilli(int64(i)>>tsShift + epochMillis)
}

// MarshalJSON 输出为字符串（JS 安全）。
func (i ID) MarshalJSON() ([]byte, error) { return json.Marshal(i.String()) }

// UnmarshalJSON 接受字符串或数字。
func (i *ID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		var n int64
		if err2 := json.Unmarshal(b, &n); err2 != nil {
			return fmt.Errorf("ids: 非法 ID: %s", string(b))
		}
		*i = ID(n)
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("ids: 非法 ID: %q", s)
	}
	*i = ID(n)
	return nil
}

// Parse 从字符串解析。
func Parse(s string) (ID, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ids: 非法 ID: %q", s)
	}
	return ID(n), nil
}
