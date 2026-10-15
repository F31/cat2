// Package matrix 权限矩阵表驱动框架（p1 §13.1）：PRD 3.2/3.3 每一格一条用例。
// 用例只描述「谁 × 做什么 × 期望」，由调用方注入探测函数（真实 HTTP 装配），
// 保证矩阵可穷举、可逐格定位失败，不依赖测试内的散落断言。
package matrix

import (
	"fmt"
	"testing"
)

// Role 访问角色（用于命名与分组统计）。
type Role string

const (
	Guest      Role = "游客"
	User       Role = "注册用户"
	Unrealname Role = "未实名"
	NonMember  Role = "非会员"
	Member     Role = "会员"
	NonAdmin   Role = "非管理员"
	Admin      Role = "管理员"
)

// Expect 期望结果。
type Expect struct {
	Status int // 非 0 时精确匹配状态码；0 表示放行（2xx）
	Code   int // 非 0 时匹配业务错误码

	anyDenied bool // 只要求拒绝（401/403）
}

// Denied 拒绝（401/403）并可指定错误码。
func Denied(status, code int) Expect { return Expect{Status: status, Code: code} }

// AnyDenied 只要求拒绝（401/403）。
func AnyDenied() Expect { return Expect{Status: 0, Code: 0, anyDenied: true} }

// Allowed 放行（2xx）。
func Allowed() Expect { return Expect{Status: 200} }

// Case 一条矩阵用例。
type Case struct {
	Name   string
	Role   Role
	Method string
	Path   string
	Token  string // 该角色令牌（游客为空）
	Body   any
	Want   Expect
}

// Probe 探测函数：返回 HTTP 状态码与业务错误码。
type Probe func(method, path, token string, body any) (status int, code int)

// Run 逐格执行并按角色分组报告。
func Run(t *testing.T, probe Probe, cases []Case) {
	t.Helper()
	byRole := map[Role]int{}
	for _, c := range cases {
		c := c
		byRole[c.Role]++
		t.Run(string(c.Role)+"/"+c.Name, func(t *testing.T) {
			status, code := probe(c.Method, c.Path, c.Token, c.Body)
			switch {
			case c.Want.anyDenied:
				if status != 401 && status != 403 {
					t.Fatalf("%s 应拒绝（401/403），实际 %d code=%d", c.Name, status, code)
				}
			case c.Want.Status >= 400:
				if status != c.Want.Status {
					t.Fatalf("%s 状态码应 %d，实际 %d code=%d", c.Name, c.Want.Status, status, code)
				}
				if c.Want.Code != 0 && code != c.Want.Code {
					t.Fatalf("%s 错误码应 %d，实际 %d", c.Name, c.Want.Code, code)
				}
			default:
				if status < 200 || status >= 300 {
					t.Fatalf("%s 应放行，实际 %d code=%d", c.Name, status, code)
				}
			}
		})
	}
	t.Logf("权限矩阵：%d 格（%s）", len(cases), summary(byRole))
}

func summary(m map[Role]int) string {
	out := ""
	for r, n := range m {
		if out != "" {
			out += ", "
		}
		out += fmt.Sprintf("%s=%d", r, n)
	}
	return out
}
