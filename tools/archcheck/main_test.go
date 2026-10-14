// archcheck 自检测试：故意违规的样例必须被检出。
package main

import "testing"

func TestSQLPrefixViolation(t *testing.T) {
	// cube 模块 SQL 访问了 content 的 cnt_ 表 → 违规
	sql := `INSERT INTO cub_task (id) VALUES ($1); SELECT * FROM cnt_item WHERE id=$2;`
	vs := checkSQLText("fixture.sql", "cube", sql)
	if len(vs) != 1 {
		t.Fatalf("期望 1 条违规，实际 %d: %v", len(vs), vs)
	}
}

func TestSQLPrefixOK(t *testing.T) {
	sql := `INSERT INTO cub_task (id, cube_id) VALUES ($1,$2); UPDATE cub_member SET status=1;`
	if vs := checkSQLText("fixture.sql", "cube", sql); len(vs) != 0 {
		t.Fatalf("不应违规: %v", vs)
	}
}

func TestSQLSysTableViolation(t *testing.T) {
	sql := `UPDATE sys_settings SET value='x'`
	if vs := checkSQLText("fixture.sql", "content", sql); len(vs) == 0 {
		t.Fatal("业务模块访问 sys_ 内核表必须违规")
	}
}

func TestRankTableComplete(t *testing.T) {
	// 依赖阶梯表与表前缀表必须覆盖同一批模块（entityref 无独立表）
	for m := range rank {
		if m == "entityref" {
			continue
		}
		if _, ok := tablePrefix[m]; !ok {
			t.Fatalf("模块 %s 缺表前缀登记", m)
		}
	}
}
