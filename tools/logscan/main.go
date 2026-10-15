// logscan 日志敏感信息扫描：静态检查日志调用点是否可能记录敏感字段（p6 §5.2）。
// 规则：日志调用行内出现敏感字段名字面量或敏感变量名，且未经脱敏函数包装 → 违规。
// 源码可用 `//logscan:ignore`（行内或上一行）显式豁免。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var loggerCall = regexp.MustCompile(`\b(slog\.(Debug|Info|Warn|Error|Log)|log\.(Print|Printf|Println|Fatal)|logger\.(Debug|Info|Warn|Error))`)

// 敏感字段名（字面量键）
var sensitiveKeys = []string{
	"session_key", "encrypted_data", "id_card_no", "id_no", "real_name", "password",
	"secret", "appsecret", "app_secret", "access_token", "refresh_token", "private_key",
	"enc_key", "contact_phone", "phone", "bank_card", "id_card",
}

// 敏感变量名（未加引号作为参数值）
var sensitiveVars = regexp.MustCompile(`\b(sessionKey|session_key|accessToken|refreshToken|idCardNo|idNo|realName|password|secret|phone|contactPhone)\b`)

// 脱敏/豁免包装
var safeWrap = regexp.MustCompile(`(mask\.|maskTail\(|obs\.Mask|redact|Redact|hash|Hash|fingerprint)`)

// stripStrings 去掉字符串字面量，避免把日志文案中的敏感词误判为字段。
var strLit = regexp.MustCompile(`"[^"]*"`)

func stripStrings(s string) string { return strLit.ReplaceAllString(s, `""`) }

func main() {
	root := flag.String("root", "../apps/server", "扫描根目录")
	flag.Parse()
	violations := scan(*root)
	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Println("logscan:", v)
		}
		fmt.Printf("logscan: 发现 %d 处疑似敏感日志\n", len(violations))
		os.Exit(1)
	}
	fmt.Println("logscan: 日志敏感信息扫描通过")
}

// scan 扫描目录下 Go 源码，返回疑似敏感日志清单。
func scan(root string) []string {
	var violations []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == "vendor" || d.Name() == "gen") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		prev, line := "", 0
		for sc.Scan() {
			cur := sc.Text()
			line++
			text := strings.TrimSpace(cur)
			if strings.Contains(cur, "//logscan:ignore") || strings.Contains(prev, "//logscan:ignore") {
				prev = cur
				continue
			}
			if strings.HasPrefix(text, "//") {
				prev = cur
				continue
			}
			if loggerCall.MatchString(cur) && !safeWrap.MatchString(cur) {
				hit := ""
				for _, k := range sensitiveKeys {
					if strings.Contains(cur, `"`+k+`"`) {
						hit = "字段 " + k
						break
					}
				}
				if hit == "" {
					if m := sensitiveVars.FindString(stripStrings(cur)); m != "" {
						hit = "变量 " + m
					}
				}
				if hit != "" {
					violations = append(violations, fmt.Sprintf("%s:%d 疑似记录敏感信息（%s）: %s", path, line, hit, text))
				}
			}
			prev = cur
		}
		return nil
	})
	return violations
}
