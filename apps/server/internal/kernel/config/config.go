// Package config 启动配置（三层配置的第一层：环境变量/文件）。
package config

import (
	"os"
	"strings"
)

// Config 启动配置。
type Config struct {
	DatabaseURL string
	HTTPAddr    string
	Roles       []string // api / worker / scheduler / admin
	Modules     []string // modules.enabled；空=全部
}

// FromEnv 读取环境变量。
func FromEnv() Config {
	return Config{
		DatabaseURL: getenv("CATCUBE_DATABASE_URL", "postgres://catcube:catcube@localhost:5432/catcube?sslmode=disable"),
		HTTPAddr:    getenv("CATCUBE_HTTP_ADDR", ":8080"),
		Roles:       split(getenv("CATCUBE_ROLES", "api,worker,scheduler,admin")),
		Modules:     split(getenv("CATCUBE_MODULES", "")),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// HasRole 是否启用某角色。
func (c Config) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}
