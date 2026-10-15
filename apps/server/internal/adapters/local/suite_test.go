package local

import (
	"testing"

	"catcube/internal/ports/suite"
)

// 端口契约：进程内缓存与实时 Noop。
func TestCacheContract(t *testing.T)     { suite.Cache(t, NewCache()) }
func TestRealtimeContract(t *testing.T)  { suite.Realtime(t, RealtimeNoop{}) }
