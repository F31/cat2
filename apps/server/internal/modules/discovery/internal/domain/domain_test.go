package domain

import "testing"

func TestEvaluateComplement(t *testing.T) {
	my := Profile{DocType: TypeUser, Region: "310000", Industry: []string{"A01"}, Need: []string{"N1"}, Offer: []string{"O1"}}
	their := Profile{DocType: TypeOrg, Region: "310000", Industry: []string{"A01"}, Need: []string{"O1"}, Offer: []string{"N1"}}
	score, reasons := Evaluate(my, their, DefaultWeights())
	// 全部维度满分：40+25+20+15 = 100
	if score != 100 {
		t.Fatalf("满匹配应 100，得 %v（%v）", score, reasons)
	}
	if len(reasons) != 4 {
		t.Fatalf("应给出 4 条理由，得 %d", len(reasons))
	}
}

func TestEvaluatePartial(t *testing.T) {
	my := Profile{DocType: TypeUser, Region: "310000", Industry: []string{"A01"}, Need: []string{"N1", "N2"}}
	their := Profile{DocType: TypeUser, Region: "310000", Industry: []string{"A02"}, Offer: []string{"N1"}}
	score, _ := Evaluate(my, their, DefaultWeights())
	// need: (0.5+0)/2=0.25 → 10; industry 同父=0.5 → 12.5; region 1 → 20; identity 同 person=0.5 → 7.5；合计 50
	if score < 49.9 || score > 50.1 {
		t.Fatalf("部分匹配应约 50，得 %v", score)
	}
}

func TestIndustryAndRegionFactors(t *testing.T) {
	if industryFactor([]string{"A01"}, []string{"A01"}) != 1 {
		t.Fatal("同行业应 1")
	}
	if industryFactor([]string{"A01"}, []string{"A02"}) != 0.5 {
		t.Fatal("同父行业应 0.5")
	}
	if industryFactor([]string{"A01"}, []string{"B01"}) != 0 {
		t.Fatal("不同行业应 0")
	}
	if regionFactor("310000", "310000") != 1 || regionFactor("310000", "310100") != 0.5 || regionFactor("310000", "110000") != 0 {
		t.Fatal("区域因子不符")
	}
	if identityFactor(TypeUser, TypeOrg) != 1 || identityFactor(TypeUser, TypeUser) != 0.5 {
		t.Fatal("身份因子不符")
	}
}

func TestSortCandidates(t *testing.T) {
	list := []Ranked{
		{Ref: "a", Score: 60, MemberLevel: 1, LastActive: 100},
		{Ref: "b", Score: 60, MemberLevel: 3, LastActive: 50},
		{Ref: "c", Score: 60, MemberLevel: 3, LastActive: 90},
		{Ref: "d", Score: 80, MemberLevel: 0, LastActive: 10},
	}
	SortCandidates(list)
	want := []string{"d", "c", "b", "a"}
	for i, ref := range want {
		if list[i].Ref != ref {
			t.Fatalf("排序不符：位置 %d 期望 %s 得 %s（%+v）", i, ref, list[i].Ref, list)
		}
	}
}
