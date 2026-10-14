package ids

import (
	"encoding/json"
	"testing"
)

func TestNewMonotonic(t *testing.T) {
	const n = 10000
	prev := New()
	for i := 0; i < n; i++ {
		cur := New()
		if cur <= prev {
			t.Fatalf("ID 非递增: prev=%d cur=%d", prev, cur)
		}
		prev = cur
	}
}

func TestJSONString(t *testing.T) {
	id := New()
	b, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	if b[0] != '"' {
		t.Fatalf("ID 必须以字符串输出 JSON: %s", b)
	}
	var back ID
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back != id {
		t.Fatalf("往返不一致: %d != %d", back, id)
	}
}

func TestTimeRoundtrip(t *testing.T) {
	id := New()
	if d := id.Time(); d.Unix() <= 0 {
		t.Fatalf("还原时间非法: %v", d)
	}
}
