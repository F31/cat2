// archcheck 边界检查（CI 必过）：
//  1. 依赖阶梯：模块只能依赖更低阶模块的 contract（shared 可被所有依赖）
//  2. SQL 表前缀：模块 infra 的 SQL 只能访问本模块前缀的表
//  3. infra 层之外禁用 database/sql / pgx 直接 import
//  4. bff 只读：bff 只能依赖模块 contract，不得 import 模块根包
package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// rank 依赖阶梯（shared 最低，可被所有人依赖）。
var rank = map[string]int{
	"dict": -1, "media": -1, "entityref": -1,
	"identity": 0, "trust": 0, "ai": 0,
	"profile": 1, "msg": 1, "trade": 1, "social": 1,
	"verify": 2, "content": 2,
	"membership": 3, "cube": 3,
	"circle": 4, "channel": 4,
	"discovery": 5,
	"notice":    6,
}

// tablePrefix 表前缀即归属。
var tablePrefix = map[string]string{
	"identity": "idn_", "trust": "trs_", "ai": "ai_",
	"dict": "dct_", "media": "med_",
	"profile": "prf_", "msg": "msg_", "trade": "trd_", "social": "soc_",
	"verify": "vfy_", "content": "cnt_",
	"membership": "mbr_", "cube": "cub_",
	"circle": "cir_", "channel": "chn_",
	"discovery": "dsc_", "notice": "ntc_",
}

var tableRefRe = regexp.MustCompile(`(?i)\b(?:FROM|INTO|UPDATE|JOIN|TABLE)\s+([a-z][a-z0-9_]*)`)

type violation struct{ file, rule, detail string }

func main() {
	root := flag.String("root", "apps/server", "后端根目录")
	flag.Parse()
	var vs []violation

	modRoot := filepath.Join(*root, "internal", "modules")
	sharedRoot := filepath.Join(*root, "internal", "shared")
	bffRoot := filepath.Join(*root, "internal", "bff")

	for _, base := range []string{modRoot, sharedRoot} {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if _, ok := rank[name]; !ok {
				vs = append(vs, violation{base, "模块登记", name + " 未在依赖阶梯表登记"})
				continue
			}
			vs = append(vs, checkModule(filepath.Join(base, name), name)...)
		}
	}
	vs = append(vs, checkBFF(bffRoot)...)

	if len(vs) > 0 {
		fmt.Println("archcheck 边界违规：")
		for _, v := range vs {
			fmt.Printf("  ✗ %s [%s] %s\n", v.file, v.rule, v.detail)
		}
		os.Exit(1)
	}
	fmt.Println("archcheck: 依赖阶梯、表前缀、infra 禁令、bff 只读全部通过")
}

// checkModule 检查一个模块目录。
func checkModule(dir, name string) []violation {
	var vs []violation
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(path, dir+string(filepath.Separator))
		inInfra := strings.HasPrefix(rel, filepath.Join("internal", "infra"))
		switch {
		case strings.HasSuffix(path, ".go"):
			vs = append(vs, checkGoFile(path, name, inInfra)...)
			// 内联 SQL 同样受表前缀约束（迁移脚本常量除外：迁移只含本模块表）
			vs = append(vs, checkInlineSQL(path, name)...)
		case strings.HasSuffix(path, ".sql"):
			if !inInfra {
				vs = append(vs, violation{path, "SQL 位置", "SQL 文件只能放在 internal/infra 下"})
				break
			}
			vs = append(vs, checkSQL(path, name)...)
		}
		return nil
	})
	return vs
}

// checkGoFile import 检查：阶梯方向 + infra 外禁用驱动 + 不得 import 其他模块 internal。
func checkGoFile(path, moduleName string, inInfra bool) []violation {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return []violation{{path, "解析", err.Error()}}
	}
	var vs []violation
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if !strings.HasPrefix(p, "catcube/") {
			// 驱动禁令：infra 之外不得 import database/sql 或 pgx
			if !inInfra && (p == "database/sql" || strings.HasPrefix(p, "github.com/jackc/pgx")) {
				vs = append(vs, violation{path, "驱动禁令", "infra 之外禁用 " + p})
			}
			continue
		}
		owner := importOwner(p)
		if owner == "" || owner == moduleName {
			continue
		}
		// 跨模块引用只能指向 contract 包；shared 轻量包（如 entityref）允许直接引用其根包
		isSharedRoot := rank[owner] == -1 && p == "catcube/internal/shared/"+owner
		if !strings.Contains(p, "/contract") && !isSharedRoot {
			vs = append(vs, violation{path, "边界", "跨模块只能 import 对方 contract 包: " + p})
			continue
		}
		// 阶梯：只能依赖更低阶
		if rank[owner] >= rank[moduleName] && rank[owner] != -1 {
			vs = append(vs, violation{path, "依赖阶梯",
				fmt.Sprintf("%s(R%d) 不得依赖 %s(R%d)", moduleName, rank[moduleName], owner, rank[owner])})
		}
	}
	return vs
}

// importOwner 返回 import 路径所属的模块名（modules/shared 下的顶层目录）。
func importOwner(p string) string {
	for _, base := range []string{"catcube/internal/modules/", "catcube/internal/shared/"} {
		if strings.HasPrefix(p, base) {
			rest := strings.TrimPrefix(p, base)
			if i := strings.Index(rest, "/"); i > 0 {
				return rest[:i]
			}
			return rest
		}
	}
	return ""
}

// checkSQL 表前缀校验。
func checkSQL(path, moduleName string) []violation {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return checkSQLText(path, moduleName, string(raw))
}

func checkSQLText(path, moduleName, text string) []violation {
	prefix := tablePrefix[moduleName]
	seen := map[string]bool{}
	var vs []violation
	for _, m := range tableRefRe.FindAllStringSubmatch(text, -1) {
		table := strings.ToLower(m[1])
		if seen[table] {
			continue
		}
		seen[table] = true
		if strings.HasPrefix(table, prefix) {
			continue
		}
		// 其他模块前缀或 sys_ 一律违规；非登记表名（如 CTE）跳过
		if isForeignTable(table, moduleName) {
			vs = append(vs, violation{path, "表前缀",
				fmt.Sprintf("访问了非本模块表 %s（本模块前缀 %s）", table, prefix)})
		}
	}
	return vs
}

// isForeignTable 判断是否属于其他模块/内核的表。
func isForeignTable(table, moduleName string) bool {
	if strings.HasPrefix(table, "sys_") {
		return true
	}
	for owner, prefix := range tablePrefix {
		if owner != moduleName && strings.HasPrefix(table, prefix) {
			return true
		}
	}
	return false
}

// checkInlineSQL 对 .go 文件中的内联 SQL 做表前缀校验。
func checkInlineSQL(path, moduleName string) []violation {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return checkSQLText(path, moduleName, string(raw))
}

// checkBFF bff 只读聚合：不得 import 模块根包（只允许 contract）。
func checkBFF(bffRoot string) []violation {
	var vs []violation
	filepath.WalkDir(bffRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if owner := importOwner(p); owner != "" && !strings.Contains(p, "/contract") {
				vs = append(vs, violation{path, "bff 只读", "bff 只能依赖模块 contract: " + p})
			}
		}
		return nil
	})
	return vs
}
