package errs

import "testing"

func TestErrorConstructors(t *testing.T) {
	if e := BadRequest("110001", "参数错误"); e.Kind != KindBadRequest || e.Code != "110001" || e.Reason != "" {
		t.Fatalf("BadRequest 构造不符: %+v", e)
	}
	if e := Forbidden("130002", "member.required"); e.Kind != KindForbidden || e.Reason != "member.required" {
		t.Fatalf("Forbidden 应带 Reason: %+v", e)
	}
	if e := RateLimited("140008", "quota.post"); e.Kind != KindRateLimited {
		t.Fatalf("RateLimited Kind: %+v", e)
	}
	if e := Internal("boom"); e.Code != "000000" {
		t.Fatalf("Internal 默认码: %+v", e)
	}
	if err := Conflict("080007", "分摊已结束"); err.Error() == "" {
		t.Fatal("Error() 不应为空")
	}
}
