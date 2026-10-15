// Package domain discovery 领域规则：匹配评分（需求供给互补/行业/区域/身份）与推荐理由模板（纯函数）。
package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// 领域错误（映射到错误码 16xxxx）。
var (
	ErrBadType     = errors.New("discovery: 未知内容类型")
	ErrNoProfile   = errors.New("discovery: 无画像")
	ErrNoCandidate = errors.New("discovery: 候选不存在")
)

// 文档类型（六类：人/企业/项目/产品/会议/活动）。
const (
	TypeUser    = "user"
	TypeOrg     = "org"
	TypeProject = "content.project"
	TypeProduct = "content.product"
	TypeMeeting = "content.meeting"
	TypeEvent   = "content.event"
)

// SearchTypes 可检索的六类。
var SearchTypes = []string{TypeUser, TypeOrg, TypeProject, TypeProduct, TypeMeeting, TypeEvent}

// ValidType 是否可检索类型。
func ValidType(t string) bool {
	for _, x := range SearchTypes {
		if x == t {
			return true
		}
	}
	return false
}

// Weights 评分权重（万分位/百分制由 SettingsSchema 配置；此处为百分制分值）。
type Weights struct {
	Need     int
	Industry int
	Region   int
	Identity int
}

// DefaultWeights 规格默认：需求供给互补 40 + 行业 25 + 区域 20 + 身份互补 15。
func DefaultWeights() Weights { return Weights{Need: 40, Industry: 25, Region: 20, Identity: 15} }

// Profile 匹配画像向量。
type Profile struct {
	DocType  string
	Region   string
	Industry []string
	Need     []string
	Offer    []string
}

// ReasonItem 推荐理由（规则模板生成，不依赖 LLM）。
type ReasonItem struct {
	Dimension string `json:"dimension"`
	Text      string `json:"text"`
}

// Evaluate 计算匹配分（0~100）与推荐理由。
func Evaluate(my, their Profile, w Weights) (float64, []ReasonItem) {
	total := w.Need + w.Industry + w.Region + w.Identity
	if total <= 0 {
		w = DefaultWeights()
		total = 100
	}
	needF := needOfferFactor(my, their)
	indF := industryFactor(my.Industry, their.Industry)
	regF := regionFactor(my.Region, their.Region)
	idF := identityFactor(my.DocType, their.DocType)

	score := float64(w.Need)*needF + float64(w.Industry)*indF + float64(w.Region)*regF + float64(w.Identity)*idF
	if total != 100 {
		score = score * 100 / float64(total)
	}

	var reasons []ReasonItem
	if needF > 0 {
		reasons = append(reasons, ReasonItem{Dimension: "need_offer", Text: fmt.Sprintf("供需互补：%d%% 契合", int(needF*100))})
	}
	if indF > 0 {
		text := "行业相关"
		if indF >= 1 {
			text = "同行业"
		}
		reasons = append(reasons, ReasonItem{Dimension: "industry", Text: text})
	}
	if regF > 0 {
		text := "区域相近"
		if regF >= 1 {
			text = "同区域"
		}
		reasons = append(reasons, ReasonItem{Dimension: "region", Text: text})
	}
	if idF > 0 {
		reasons = append(reasons, ReasonItem{Dimension: "identity", Text: "身份互补"})
	}
	return round2(score), reasons
}

// needOfferFactor 需求供给互补：我的需求被对方供给覆盖 + 我的供给满足对方需求，各占一半。
func needOfferFactor(my, their Profile) float64 {
	a := overlapRatio(my.Need, their.Offer)
	b := overlapRatio(my.Offer, their.Need)
	return (a + b) / 2
}

// overlapRatio |a∩b|/|a|（a 空则 0）。
func overlapRatio(a, b []string) float64 {
	if len(a) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	n := 0
	for _, x := range a {
		if set[x] {
			n++
		}
	}
	return float64(n) / float64(len(a))
}

// industryFactor 同子行业 > 同行业：完全一致=1；同父级（前 2 位）=0.5；否则 0。
func industryFactor(a, b []string) float64 {
	best := 0.0
	for _, x := range a {
		for _, y := range b {
			if x == y {
				if 1.0 > best {
					best = 1.0
				}
			} else if parent(x) == parent(y) && parent(x) != "" {
				if 0.5 > best {
					best = 0.5
				}
			}
		}
	}
	return best
}

// regionFactor 完全一致=1；同省（前 2 位）=0.5；否则 0。
func regionFactor(a, b string) float64 {
	switch {
	case a == "" || b == "":
		return 0
	case a == b:
		return 1
	case parent(a) == parent(b) && parent(a) != "":
		return 0.5
	default:
		return 0
	}
}

// identityFactor 身份互补：个人↔企业互补=1；否则 0.5。
func identityFactor(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if isPerson(a) != isPerson(b) {
		return 1
	}
	return 0.5
}

func isPerson(t string) bool { return t == TypeUser }

func parent(code string) string {
	if len(code) >= 2 {
		return code[:2]
	}
	return code
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// SortCandidates 稳定排序：分数高→会员等级高→近期活跃；rank 从 1 起。
type Ranked struct {
	Ref         string
	Score       float64
	Reason      []ReasonItem
	MemberLevel int16
	LastActive  int64 // unix 秒
}

func SortCandidates(list []Ranked) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Score != list[j].Score {
			return list[i].Score > list[j].Score
		}
		if list[i].MemberLevel != list[j].MemberLevel {
			return list[i].MemberLevel > list[j].MemberLevel
		}
		return list[i].LastActive > list[j].LastActive
	})
}

// SplitRef 拆分 EntityRef "type:id"。
func SplitRef(ref string) (typ string, id int64, ok bool) {
	i := strings.LastIndex(ref, ":")
	if i <= 0 || i == len(ref)-1 {
		return "", 0, false
	}
	var n int64
	if _, err := fmt.Sscanf(ref[i+1:], "%d", &n); err != nil {
		return "", 0, false
	}
	return ref[:i], n, true
}
