// Package realname 实名二要素核验适配器。
package realname

import (
	"context"
	"regexp"
	"strings"
)

// DevClient 开发态二要素核验：仅做格式校验后按约定返回一致/不一致，
// 绝不用于生产（装配根按配置选择真实供应商实现）。
// 约定：姓名含「驳回」视为不一致（便于本地与 CI 端到端验证失败分支）。
type DevClient struct{}

func NewDevClient() *DevClient { return &DevClient{} }

var idCardRe = regexp.MustCompile(`^\d{17}[\dXx]$`)

// Verify 开发态：格式合法且姓名不含「驳回」即视为核验通过。
func (d *DevClient) Verify(_ context.Context, name, idCardNo string) (bool, error) {
	name = strings.TrimSpace(name)
	idCardNo = strings.TrimSpace(idCardNo)
	if name == "" || !idCardRe.MatchString(idCardNo) {
		return false, nil
	}
	if strings.Contains(name, "驳回") {
		return false, nil
	}
	return true, nil
}
