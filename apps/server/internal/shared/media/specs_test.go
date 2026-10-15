package media

import "testing"

func TestResolveSpec(t *testing.T) {
	if s, err := ResolveSpec(""); err != nil || s.ProviderOps != "" {
		t.Fatalf("空规格应为原图: %+v %v", s, err)
	}
	s, err := ResolveSpec("thumb_s")
	if err != nil || s.MaxEdge != 200 || s.ProviderOps == "" {
		t.Fatalf("thumb_s 规格不正确: %+v %v", s, err)
	}
	if _, err := ResolveSpec("thumb_xl"); err == nil {
		t.Fatal("未知规格必须报错（防静默返回原图）")
	}
	if len(SpecNames()) < 5 {
		t.Fatalf("规格集过少: %v", SpecNames())
	}
}
