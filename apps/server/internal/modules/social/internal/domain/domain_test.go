package domain

import "testing"

func TestParseRef(t *testing.T) {
	cases := []struct {
		in      string
		typ     string
		id      int64
		wantErr bool
	}{
		{"user:123", "user", 123, false},
		{"content.project:456", "content.project", 456, false},
		{"org:1", "org", 1, false},
		{"badref", "", 0, true},
		{"user:0", "", 0, true},
		{"user:abc", "", 0, true},
	}
	for _, c := range cases {
		r, err := ParseRef(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("%q 应报错", c.in)
			}
			continue
		}
		if err != nil || r.Type != c.typ || r.ID != c.id {
			t.Fatalf("ParseRef(%q)=%+v err=%v", c.in, r, err)
		}
	}
}

func TestKindOf(t *testing.T) {
	cases := map[string]string{
		"user":            KindPerson,
		"org":             KindCompany,
		"content.project": KindProject,
		"content.product": KindProduct,
		"content.meeting": KindMeeting,
		"content.event":   KindEvent,
	}
	for refType, want := range cases {
		got, ok := KindOf(refType)
		if !ok || got != want {
			t.Fatalf("KindOf(%q)=%q,%v want %q", refType, got, ok, want)
		}
	}
	if _, ok := KindOf("unknown"); ok {
		t.Fatal("未知类型不应有分类")
	}
}

func TestValidKind(t *testing.T) {
	if !ValidKind("") || !ValidKind("project") {
		t.Fatal("空与非空合法分类均应通过")
	}
	if ValidKind("bogus") {
		t.Fatal("非法分类应拒绝")
	}
}
