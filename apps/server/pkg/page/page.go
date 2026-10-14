// Package page 游标分页（默认每页 20 条）。
package page

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const DefaultSize = 20

// Cursor 分页游标，承载上一页最后一条的 int64 主键。
type Cursor struct {
	LastID int64
	Size   int
}

// Encode 序列化为不透明字符串。
func (c Cursor) Encode() string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%d", c.LastID, c.Size)))
}

// Decode 解析游标；空字符串表示第一页。
func Decode(s string, size int) (Cursor, error) {
	if size <= 0 {
		size = DefaultSize
	}
	if s == "" {
		return Cursor{Size: size}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("page: 非法游标")
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return Cursor{}, fmt.Errorf("page: 非法游标")
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Cursor{}, fmt.Errorf("page: 非法游标")
	}
	return Cursor{LastID: id, Size: size}, nil
}

// Result 分页结果；Next 为空表示没有下一页。
type Result[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next,omitempty"`
}
