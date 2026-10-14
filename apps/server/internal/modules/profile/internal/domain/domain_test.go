package domain

import (
	"reflect"
	"testing"
)

func secondLayerCard() Card {
	return Card{
		IdentityType:  1,
		Name:          "张三",
		RegionCode:    "310000",
		IndustryCodes: []string{"A01"},
		NeedCodes:     []string{"N1"},
		OfferCodes:    []string{"O1"},
		Intro:         "专注产业合作",
	}
}

func TestCompletion(t *testing.T) {
	// 第二层五项全有 → 完成，tier=2
	missing, complete, tier := Completion(secondLayerCard())
	if !complete || len(missing) != 0 || tier != 2 {
		t.Fatalf("五项齐全应完成/tier2: missing=%v complete=%v tier=%d", missing, complete, tier)
	}

	// 缺合作需求 → 未完成，缺失项稳定含 need
	c := secondLayerCard()
	c.NeedCodes = nil
	missing, complete, _ = Completion(c)
	if complete || !reflect.DeepEqual(missing, []string{"need"}) {
		t.Fatalf("缺 need 应未完成: %v %v", missing, complete)
	}

	// 全空 → 五项按稳定顺序缺失
	missing, complete, tier = Completion(Card{IdentityType: 1, Name: "x"})
	if complete || tier != 1 {
		t.Fatalf("全空应未完成/tier1: %v %d", complete, tier)
	}
	want := []string{"region", "industry", "need", "offer", "intro"}
	if !reflect.DeepEqual(missing, want) {
		t.Fatalf("缺失顺序不符: got=%v want=%v", missing, want)
	}

	// 第二层完整 + 增值画像 → tier=3
	c = secondLayerCard()
	c.CoreStrength = "核心优势"
	if _, _, tier = Completion(c); tier != 3 {
		t.Fatalf("含增值画像应 tier3: %d", tier)
	}
	// 仅填联系方式也算增值
	c = secondLayerCard()
	c.HasContact = true
	if _, _, tier = Completion(c); tier != 3 {
		t.Fatalf("配置联系方式应 tier3: %d", tier)
	}
}

func TestCompletionIndependentOfRealname(t *testing.T) {
	// 完成度只看第二层，与实名无关（本包无实名字段，天然满足）
	if _, complete, _ := Completion(secondLayerCard()); !complete {
		t.Fatal("第二层完整应完成，不应受实名影响")
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(Card{Name: "  "}); err != ErrNameRequired {
		t.Fatalf("名称必填: %v", err)
	}
	longIntro := make([]rune, maxIntroPersonal+1)
	for i := range longIntro {
		longIntro[i] = '字'
	}
	if err := Validate(Card{Name: "张三", IdentityType: 1, Intro: string(longIntro)}); err != ErrIntroTooLong {
		t.Fatalf("个人简介超长应拒绝: %v", err)
	}
	// 组织上限更高
	if err := Validate(Card{Name: "某公司", IdentityType: 2, Intro: string(longIntro)}); err != nil {
		t.Fatalf("组织简介未超长不应拒绝: %v", err)
	}
	strength := make([]rune, maxCoreStrength+1)
	for i := range strength {
		strength[i] = 'x'
	}
	if err := Validate(Card{Name: "张三", IdentityType: 1, CoreStrength: string(strength)}); err != ErrStrengthTooLong {
		t.Fatalf("核心优势超长应拒绝: %v", err)
	}
}

func TestLockedChange(t *testing.T) {
	old := Card{Name: "旧名称", RegionCode: "310000", SubjectTypeCode: "company"}
	locked := []string{"name", "subject_type_code"}
	if got := LockedChange(old, old, locked); got != "" {
		t.Fatalf("未改动不应命中锁定: %q", got)
	}
	changed := old
	changed.Name = "新名称"
	if got := LockedChange(old, changed, locked); got != "name" {
		t.Fatalf("改名称应命中锁定: %q", got)
	}
	// 未锁定的字段可改
	changed = old
	changed.RegionCode = "110000"
	if got := LockedChange(old, changed, locked); got != "" {
		t.Fatalf("未锁定字段不应拦截: %q", got)
	}
}

func TestPublicFilter(t *testing.T) {
	c := Card{Name: "张三", Intro: "简介", CoreStrength: "优势", NeedCodes: []string{"N1"}}
	// 缺省可见；联系方式永不下发
	pub := Public(c, nil)
	if pub.CoreStrength != "优势" || pub.Intro != "简介" || pub.HasContact {
		t.Fatalf("缺省应可见且联系方式不输出: %+v", pub)
	}
	// 关闭 intro 与 need
	pub = Public(c, Visibility{"intro": false, "need": false})
	if pub.Intro != "" || pub.NeedCodes != nil {
		t.Fatalf("关闭字段应被过滤: %+v", pub)
	}
	if pub.CoreStrength != "优势" {
		t.Fatalf("未关闭字段应保留: %+v", pub)
	}
}

func TestSplitJoinCodes(t *testing.T) {
	got := SplitCodes("A01, A02 ,,A03")
	if !reflect.DeepEqual(got, []string{"A01", "A02", "A03"}) {
		t.Fatalf("分割去空不符: %v", got)
	}
	if JoinCodes([]string{"A01", " ", "A02"}) != "A01,A02" {
		t.Fatalf("拼接去空不符: %q", JoinCodes([]string{"A01", " ", "A02"}))
	}
	if SplitCodes("") != nil || JoinCodes(nil) != "" {
		t.Fatal("空输入应得空结果")
	}
}
