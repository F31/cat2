package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"catcube/internal/adapters/llm"
	"catcube/internal/modules/ai/contract"
)

// AI 固定样本集评测（p5 §8.1）：抽取准确率、TagSet 合法率、降级路径。
// 作为 CI 门禁（`go test ./...` 内运行，无需 DB）：提示词/收敛逻辑变更若导致指标下降即失败。
// 真实供应商接入后，用同一套样例与阈值以供应商适配器复跑。

type evalSample struct {
	name      string
	feature   string
	kind      string
	input     string
	degrade   bool
	wantField []string // 期望至少命中其一（抽取准确率）
}

var evalSamples = []evalSample{
	{name: "onboard-org", feature: "onboard", input: "我是华东重工采购总监，寻找钣金供应商", wantField: []string{"real_name", "intro"}},
	{name: "onboard-person", feature: "onboard", input: "张三 供应链顾问 十年经验", wantField: []string{"real_name", "intro"}},
	{name: "publish-project", feature: "publish", kind: "project", input: "华东重工寻钣金供应商，预算 100 万", wantField: []string{"intro", "core_strength"}},
	{name: "publish-product", feature: "publish", kind: "product", input: "工业级激光切割机，精度 0.1mm", wantField: []string{"intro", "core_strength"}},
	{name: "degrade-upstream", feature: "onboard", input: "上游故障 [FAULT] 模拟", degrade: true},
}

// 与字段池白名单一致的允许字段（评测用最小集合）。
var evalAllowed = map[string]bool{"real_name": true, "intro": true, "core_strength": true, "background": true}

func evalPrompt(feature, kind, input string) string {
	p := defaultPrompt(feature)
	if kind != "" {
		p += "（kind=" + kind + "）"
	}
	return p + "\n文本:" + input + "\n"
}

func TestAIEvalSuite(t *testing.T) {
	ctx := context.Background()
	client := llm.NewDevClient()

	var extractTotal, extractHit int
	var degradeTotal, degradeHit int
	for _, s := range evalSamples {
		raw, err := client.Complete(ctx, evalPrompt(s.feature, s.kind, s.input), nil)
		if s.degrade {
			degradeTotal++
			if err != nil {
				degradeHit++
			}
			continue
		}
		if err != nil {
			t.Fatalf("样例 %s 不应降级: %v", s.name, err)
		}
		var parsed struct {
			Title  string              `json:"title"`
			Fields []map[string]string `json:"fields"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("样例 %s 输出非合法 JSON: %v", s.name, err)
		}
		fields := make([]contract.Field, 0, len(parsed.Fields))
		for _, f := range parsed.Fields {
			fields = append(fields, contract.Field{Field: f["field"], Value: f["value"]})
		}
		kept, _ := dropUnknownFields(fields, evalAllowed)
		extractTotal++
		for _, f := range kept {
			for _, w := range s.wantField {
				if f.Field == w && f.Value != "" {
					extractHit++
				}
			}
		}
	}

	// 抽取准确率：草稿样例至少命中一个期望字段
	extractAcc := float64(extractHit) / float64(extractTotal)
	if extractAcc < 0.9 {
		t.Fatalf("抽取准确率不达标：%.2f < 0.9", extractAcc)
	}
	if degradeTotal == 0 || degradeHit != degradeTotal {
		t.Fatalf("降级路径应 100%%：%d/%d", degradeHit, degradeTotal)
	}
}

// TagSet 合法率：搜索类型/审核分类/风险等级须收敛到合法集合。
func TestAIEvalTagSetLegality(t *testing.T) {
	ctx := context.Background()
	client := llm.NewDevClient()

	// 搜索：类型必须合法（幻觉丢弃由 validSearchType 判定）
	total, legal := 0, 0
	for _, q := range []string{"上海找钣金供应商", "北京 3D 打印服务商", "深圳工业设计公司"} {
		raw, err := client.Complete(ctx, evalPrompt("search", "", q), nil)
		if err != nil {
			t.Fatalf("搜索样例降级: %v", err)
		}
		var parsed struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &parsed)
		total++
		if parsed.Type == "" || validSearchType(parsed.Type) {
			legal++
		}
	}

	// 审核：分类在合法集合、风险 0-3
	catOK := map[string]bool{"fraud": true, "ad": true, "abuse": true, "porn": true, "other": true}
	for _, s := range []string{"疑似虚假项目招商", "疑似引流广告", "疑似辱骂内容"} {
		raw, err := client.Complete(ctx, evalPrompt("review", "", s), nil)
		if err != nil {
			t.Fatalf("审核样例降级: %v", err)
		}
		var parsed struct {
			Category string `json:"category"`
			Risk     int    `json:"risk"`
		}
		_ = json.Unmarshal(raw, &parsed)
		total++
		if catOK[parsed.Category] && parsed.Risk >= 0 && parsed.Risk <= 3 {
			legal++
		}
	}

	if rate := float64(legal) / float64(total); rate < 0.9 {
		t.Fatalf("TagSet 合法率不达标：%.2f < 0.9", rate)
	}
}

// 隐私红线在评测集上同样成立：提示词不得包含手机号/证件号原文。
func TestAIEvalPrivacy(t *testing.T) {
	clean, removed := sanitizeForPrompt("张三 13812345678 证件110101199003070019 邮箱a@b.com")
	if removed == 0 {
		t.Fatal("应剥离敏感信息")
	}
	for _, bad := range []string{"13812345678", "110101199003070019", "a@b.com"} {
		if strings.Contains(clean, bad) {
			t.Fatalf("提示词仍含敏感信息 %q：%s", bad, clean)
		}
	}
}
