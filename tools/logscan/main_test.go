package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan(t *testing.T) {
	dir := t.TempDir()
	bad := `package x

import "log/slog"

func f(sessionKey, phone string) {
	slog.Info("login", "session_key", sessionKey, "phone", phone)
}
`
	good := `package x

import (
	"log/slog"

	"catcube/pkg/mask"
)

func f(phone string) {
	slog.Info("login", "phone", mask.Phone(phone))
}
`
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "good.go"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	vs := scan(dir)
	if len(vs) == 0 {
		t.Fatal("应检出违规日志")
	}
	for _, v := range vs {
		if filepath.Base(v) == "good.go" {
			t.Fatalf("脱敏调用不应告警: %s", v)
		}
	}
}

func TestScanClean(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("package x\n\nfunc f() {}\n"), 0o644)
	if vs := scan(dir); len(vs) != 0 {
		t.Fatalf("无日志不应告警: %v", vs)
	}
}
