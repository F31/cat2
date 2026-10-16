package app

import (
	"context"

	aicontract "catcube/internal/modules/ai/contract"
	trustcontract "catcube/internal/modules/trust/contract"
)

// aiSuggester 把 ai 模块适配为 trust 的审核辅助提供者（装配根桥接；两模块同阶不互相依赖）。
type aiSuggester struct{ ai aicontract.Facade }

func (s aiSuggester) Suggest(ctx context.Context, operatorID int64, subjectType string, subjectID int64, text string) trustcontract.Suggestion {
	// 未开启/上游不可用/配额用尽：返回「无建议」，审核流程不受影响
	if !s.ai.Enabled(ctx, operatorID, aicontract.FlagReview) {
		return trustcontract.Suggestion{Available: false}
	}
	res, err := s.ai.ReviewSuggestion(ctx, operatorID, subjectType, subjectID, text)
	if err != nil {
		return trustcontract.Suggestion{Available: false}
	}
	return trustcontract.Suggestion{Available: true, Category: res.Category, Risk: res.Risk, Summary: res.Summary}
}
