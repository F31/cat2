package oss

import (
	"testing"

	"catcube/internal/ports/suite"
)

// 端口契约：对象存储（未配置 endpoint 时本地 FS 回退）。
func TestStorageContract(t *testing.T) {
	t.Setenv("CATCUBE_OSS_DIR", t.TempDir())
	suite.Storage(t, NewClient("", "default", "", "", ""))
}
