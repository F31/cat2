package verify

import (
	"log/slog"
)

// logErr 记录审计/审核类错误（供排障；不改变对外响应）。
func logErr(action string, err error) {
	if err != nil {
		slog.Error("verify: "+action, "err", err)
	}
}
