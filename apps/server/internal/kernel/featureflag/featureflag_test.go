package featureflag

import "testing"

func TestEval(t *testing.T) {
	// 关闭一律 false
	if Eval(Flag{Key: "x", Enabled: false}, 1) {
		t.Fatal("关闭应 false")
	}
	// 开启无规则 = 全量
	if !Eval(Flag{Key: "x", Enabled: true}, 42) {
		t.Fatal("无规则应全量")
	}
	// 白名单
	f := Flag{Key: "x", Enabled: true, Rules: Rules{Users: []int64{7, 9}}}
	if !Eval(f, 7) || Eval(f, 8) {
		t.Fatal("白名单命中/未命中不符")
	}
	// 比例：同一用户稳定；遍历覆盖 0/100
	if Eval(Flag{Key: "x", Enabled: true, Rules: Rules{Ratio: 100}}, 1) == false {
		t.Fatal("ratio=100 应全开")
	}
	g := Flag{Key: "x", Enabled: true, Rules: Rules{Ratio: 50}}
	first := Eval(g, 12345)
	for i := 0; i < 5; i++ {
		if Eval(g, 12345) != first {
			t.Fatal("同一用户应稳定（不抖动）")
		}
	}
	on := 0
	for uid := int64(0); uid < 1000; uid++ {
		if Eval(g, uid) {
			on++
		}
	}
	if on < 400 || on > 600 {
		t.Fatalf("50%% 比例放量分布异常: %d/1000", on)
	}
}
