//go:build integration

package app

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"catcube/internal/kernel/config"
	"catcube/internal/kernel/settings"
)

func TestAdminPasswordChange(t *testing.T) {
	phone := randPhone()
	oldPassword := "OldPass12345"
	newPassword := "NewPass67890"
	hash, err := HashAdminPassword(oldPassword)
	if err != nil {
		t.Fatal(err)
	}
	h := newTestAppCfg(t, func(c *config.Config) {
		c.AdminPhone = phone
		c.AdminPasswordHash = hash
	})
	if _, err := h.app.Pool.Exec(context.Background(),
		`DELETE FROM sys_settings WHERE scope='platform' AND key=$1`, settings.AdminPasswordHashKey); err != nil {
		t.Fatal(err)
	}

	if status, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{
		"code": "wx-admin-password-" + phone, "phone_code": phone,
	}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("创建管理员用户失败: %d %+v", status, env)
	}

	status, env := h.do(http.MethodPost, "/api/admin/v1/auth/password", "", map[string]string{
		"phone": phone, "password": oldPassword,
	})
	var login struct {
		UserID int64 `json:"user_id,string"`
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(env.Data, &login)
	if status != http.StatusOK || env.Code != 0 || login.Tokens.AccessToken == "" {
		t.Fatalf("旧密码登录失败: %d %+v", status, env)
	}

	if status, _ := h.do(http.MethodPost, "/api/admin/v1/auth/password/change", login.Tokens.AccessToken, map[string]string{
		"old_password": "wrong-password", "new_password": newPassword,
	}); status != http.StatusUnauthorized {
		t.Fatalf("旧密码错误应 401，得到 %d", status)
	}
	if status, _ := h.do(http.MethodPost, "/api/admin/v1/auth/password/change", login.Tokens.AccessToken, map[string]string{
		"old_password": oldPassword, "new_password": "short",
	}); status != http.StatusBadRequest {
		t.Fatalf("过短新密码应 400，得到 %d", status)
	}

	status, env = h.do(http.MethodPost, "/api/admin/v1/auth/password/change", login.Tokens.AccessToken, map[string]string{
		"old_password": oldPassword, "new_password": newPassword,
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("修改密码失败: %d %+v", status, env)
	}

	if status, _ := h.do(http.MethodPost, "/api/admin/v1/auth/password", "", map[string]string{
		"phone": phone, "password": oldPassword,
	}); status != http.StatusUnauthorized {
		t.Fatalf("旧密码修改后应登录失败，得到 %d", status)
	}

	status, env = h.do(http.MethodPost, "/api/admin/v1/auth/password", "", map[string]string{
		"phone": phone, "password": newPassword,
	})
	_ = json.Unmarshal(env.Data, &login)
	if status != http.StatusOK || env.Code != 0 || login.Tokens.AccessToken == "" {
		t.Fatalf("新密码登录失败: %d %+v", status, env)
	}
}

func TestAdminPasswordResetBySMS(t *testing.T) {
	phone := randPhone()
	oldPassword := "OldPass12345"
	newPassword := "NewPass67890"
	hash, err := HashAdminPassword(oldPassword)
	if err != nil {
		t.Fatal(err)
	}
	h := newTestAppCfg(t, func(c *config.Config) {
		c.AdminPhone = phone
		c.AdminPasswordHash = hash
	})
	if _, err := h.app.Pool.Exec(context.Background(),
		`DELETE FROM sys_settings WHERE scope='platform' AND key=$1`, settings.AdminPasswordHashKey); err != nil {
		t.Fatal(err)
	}

	if status, env := h.do(http.MethodPost, "/api/mp/v1/auth/wechat", "", map[string]string{
		"code": "wx-admin-reset-" + phone, "phone_code": phone,
	}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("创建管理员用户失败: %d %+v", status, env)
	}

	if status, env := h.do(http.MethodPost, "/api/admin/v1/auth/password/reset/code", "", map[string]string{
		"phone": phone,
	}); status != http.StatusOK || env.Code != 0 {
		t.Fatalf("发送找回密码验证码失败: %d %+v", status, env)
	}

	if status, _ := h.do(http.MethodPost, "/api/admin/v1/auth/password/reset", "", map[string]string{
		"phone": phone, "code": "000000", "new_password": newPassword,
	}); status != http.StatusBadRequest {
		t.Fatalf("错误验证码应 400，得到 %d", status)
	}

	status, env := h.do(http.MethodPost, "/api/admin/v1/auth/password/reset", "", map[string]string{
		"phone": phone, "code": "123456", "new_password": newPassword,
	})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("短信重置密码失败: %d %+v", status, env)
	}

	if status, _ := h.do(http.MethodPost, "/api/admin/v1/auth/password", "", map[string]string{
		"phone": phone, "password": oldPassword,
	}); status != http.StatusUnauthorized {
		t.Fatalf("旧密码重置后应登录失败，得到 %d", status)
	}

	status, env = h.do(http.MethodPost, "/api/admin/v1/auth/password", "", map[string]string{
		"phone": phone, "password": newPassword,
	})
	var login struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(env.Data, &login)
	if status != http.StatusOK || env.Code != 0 || login.Tokens.AccessToken == "" {
		t.Fatalf("新密码登录失败: %d %+v", status, env)
	}
}
