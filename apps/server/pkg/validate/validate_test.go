package validate

import "testing"

func TestErrors(t *testing.T) {
	var e Errors
	e.Required("标题", "")
	e.MaxRunes("标题", "一二三四五", 3)
	if e.OK() {
		t.Fatal("应有错误")
	}
	f, ok := e.First()
	if !ok || f.Name != "标题" {
		t.Fatalf("First 定位不符: %+v %v", f, ok)
	}
	if len(e) != 2 {
		t.Fatalf("应收集 2 条: %+v", e)
	}
	var ok2 Errors
	ok2.Required("名称", "有效")
	ok2.MaxRunes("名称", "短", 10)
	if !ok2.OK() {
		t.Fatalf("应全部通过: %+v", ok2)
	}
	if _, ok := ok2.First(); ok {
		t.Fatal("无错误时 First 应返回 false")
	}
}
