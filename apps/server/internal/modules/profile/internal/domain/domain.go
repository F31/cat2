// Package domain profile 领域规则：画像完成度、字段校验、对外展示过滤、认证锁定（纯函数，无 IO）。
package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// 领域错误（映射到错误码 06xxxx）。
var (
	ErrNotFound        = errors.New("profile: 名片不存在")
	ErrNameRequired    = errors.New("profile: 名称必填")
	ErrIntroTooLong    = errors.New("profile: 简介超长")
	ErrStrengthTooLong = errors.New("profile: 核心优势超长")
	ErrFieldLocked     = errors.New("profile: 字段已被认证锁定，不可修改")
	ErrInvalidTag      = errors.New("profile: 非法标签")
)

// 第二层匹配画像五项：任一为空即未完成（与是否实名无关）。
const (
	FieldRegion   = "region"
	FieldIndustry = "industry"
	FieldNeed     = "need"
	FieldOffer    = "offer"
	FieldIntro    = "intro"
)

// secondLayer 第二层字段顺序（稳定输出，供「完善 X 项」提示）。
var secondLayer = []string{FieldRegion, FieldIndustry, FieldNeed, FieldOffer, FieldIntro}

// Card 领域名片（去 IO 的纯数据视图）。
type Card struct {
	IdentityType    int16
	SubjectTypeCode string
	Name            string
	RegionCode      string
	IndustryCodes   []string
	NeedCodes       []string
	OfferCodes      []string
	AdvantageField  string
	Background      string
	TargetCustomers string
	TargetChannels  string
	CoreStrength    string
	Intro           string
	HasContact      bool
}

// 长度约束（PRD 8.9；按字符计）。
const (
	maxIntroPersonal = 500
	maxIntroOrg      = 5000
	maxCoreStrength  = 2000
)

// Completion 计算完成度：返回缺失项（稳定顺序）、是否完成、层级。
// 第三层增值画像：优势领域/背景身份/目标客户/目标渠道/核心优势/联系方式任一有值。
func Completion(c Card) (missing []string, complete bool, tier int16) {
	if strings.TrimSpace(c.RegionCode) == "" {
		missing = append(missing, FieldRegion)
	}
	if len(nonEmpty(c.IndustryCodes)) == 0 {
		missing = append(missing, FieldIndustry)
	}
	if len(nonEmpty(c.NeedCodes)) == 0 {
		missing = append(missing, FieldNeed)
	}
	if len(nonEmpty(c.OfferCodes)) == 0 {
		missing = append(missing, FieldOffer)
	}
	if strings.TrimSpace(c.Intro) == "" {
		missing = append(missing, FieldIntro)
	}
	complete = len(missing) == 0
	tier = 1
	if complete {
		tier = 2
	}
	if hasThirdLayer(c) {
		tier = 3
	}
	return missing, complete, tier
}

func hasThirdLayer(c Card) bool {
	return strings.TrimSpace(c.AdvantageField) != "" ||
		strings.TrimSpace(c.Background) != "" ||
		strings.TrimSpace(c.TargetCustomers) != "" ||
		strings.TrimSpace(c.TargetChannels) != "" ||
		strings.TrimSpace(c.CoreStrength) != "" ||
		c.HasContact
}

// Validate 名片字段校验（按 identity_type 区分简介长度）。
func Validate(c Card) error {
	if strings.TrimSpace(c.Name) == "" {
		return ErrNameRequired
	}
	limit := maxIntroPersonal
	if c.IdentityType == 2 {
		limit = maxIntroOrg
	}
	if utf8.RuneCountInString(c.Intro) > limit {
		return ErrIntroTooLong
	}
	if utf8.RuneCountInString(c.CoreStrength) > maxCoreStrength {
		return ErrStrengthTooLong
	}
	return nil
}

// LockedChange 判断本次更新是否改动了被认证锁定的字段（old/new 转小写去空格比较）。
func LockedChange(old, new Card, locked []string) string {
	for _, f := range locked {
		switch f {
		case "name":
			if old.Name != new.Name {
				return "name"
			}
		case "subject_type_code":
			if old.SubjectTypeCode != new.SubjectTypeCode {
				return "subject_type_code"
			}
		case "region_code":
			if old.RegionCode != new.RegionCode {
				return "region_code"
			}
		}
	}
	return ""
}

// Visibility 字段级对外展示开关：键缺失或 true 表示展示，显式 false 才隐藏。
type Visibility map[string]bool

// Public 生成对外只读视图：显式关闭展示的字段不下发；联系方式明文永不下发。
func Public(c Card, v Visibility) Card {
	show := func(key string) bool {
		val, ok := v[key]
		if !ok {
			return true // 缺省可见
		}
		return val
	}
	out := c
	out.HasContact = false
	if !show("industry") {
		out.IndustryCodes = nil
	}
	if !show("need") {
		out.NeedCodes = nil
	}
	if !show("offer") {
		out.OfferCodes = nil
	}
	if !show("advantage_field") {
		out.AdvantageField = ""
	}
	if !show("background") {
		out.Background = ""
	}
	if !show("target_customers") {
		out.TargetCustomers = ""
	}
	if !show("target_channels") {
		out.TargetChannels = ""
	}
	if !show("core_strength") {
		out.CoreStrength = ""
	}
	if !show("intro") {
		out.Intro = ""
	}
	return out
}

// SplitCodes 逗号分隔 code → 去空切片。
func SplitCodes(s string) []string { return nonEmpty(strings.Split(s, ",")) }

// JoinCodes 切片 → 逗号分隔。
func JoinCodes(codes []string) string { return strings.Join(nonEmpty(codes), ",") }

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}
