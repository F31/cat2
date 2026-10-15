package pay

import (
	"testing"

	"catcube/internal/ports/suite"
)

// 端口契约：支付（开发态）。
func TestPayContract(t *testing.T) { suite.Pay(t, NewDevClient()) }
