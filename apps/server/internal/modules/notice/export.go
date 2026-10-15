package notice

import (
	"fmt"

	"catcube/internal/modules/notice/internal/app"
)

// TemplateCodes 订阅的全部事件 Topic（契约测试用）。
func TemplateCodes() []string { return app.Topics() }

// ValidateTemplates 校验模板登记表完整（≥36 条、均有接收人规则与渠道）。CI/契约测试调用。
func ValidateTemplates() error {
	if len(app.Templates) < 36 {
		return fmt.Errorf("notice: 模板数应 ≥36，实际 %d", len(app.Templates))
	}
	if len(app.Templates) != len(app.Topics()) {
		return fmt.Errorf("notice: 模板数与订阅事件数不一致")
	}
	for _, tpl := range app.Templates {
		if tpl.ReceiverRule == "" {
			return fmt.Errorf("notice: 模板 %s 缺接收人规则", tpl.EventCode)
		}
		if len(tpl.Channels) == 0 {
			return fmt.Errorf("notice: 模板 %s 缺渠道", tpl.EventCode)
		}
	}
	return nil
}
