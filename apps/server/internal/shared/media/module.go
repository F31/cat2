// Package media 共享业务域模块：统一上传、缩略图、引用计数与孤儿清理。
package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/module"
	"catcube/internal/ports"
	"catcube/internal/shared/media/contract"
	"catcube/pkg/ids"
)

// 允许的文件类型与大小（PRD 8.9.2）。
var allowedMime = map[string]bool{"image/jpeg": true, "image/jpg": true, "image/png": true}

const maxSizeBytes = 5 << 20 // 5MB

// Module media 模块。
type Module struct {
	pool    *db.Pool
	storage ports.Storage
}

func New(pool *db.Pool, storage ports.Storage) *Module {
	return &Module{pool: pool, storage: storage}
}

func (m *Module) Name() string        { return "media" }
func (m *Module) Rank() module.Rank   { return module.RankShared }
func (m *Module) DependsOn() []string { return nil }

// Register 注册上传接口。
func (m *Module) Register(r *module.Registry) error {
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/media/uploads", Handler: m.handleUpload},
	)
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

// Upload 申请预签名直传。
func (m *Module) Upload(ctx context.Context, mime string, sizeBytes int64, purpose string) (contract.UploadTicket, error) {
	if !allowedMime[mime] {
		return contract.UploadTicket{}, fmt.Errorf("media: 仅允许 JPG/JPEG/PNG")
	}
	if sizeBytes > maxSizeBytes {
		return contract.UploadTicket{}, fmt.Errorf("media: 文件大小不能超过 5MB")
	}
	id := ids.New()
	objectKey := fmt.Sprintf("%s/%d", purpose, id)
	if _, err := m.pool.Exec(ctx,
		`INSERT INTO med_file (id, bucket, object_key, mime, size_bytes, ref_count, status, created_at)
		 VALUES ($1,'default',$2,$3,$4,0,1,now())`, int64(id), objectKey, mime, sizeBytes); err != nil {
		return contract.UploadTicket{}, err
	}
	url, err := m.storage.PresignUpload(ctx, "default", objectKey, mime, 15*time.Minute)
	if err != nil {
		return contract.UploadTicket{}, err
	}
	return contract.UploadTicket{FileID: int64(id), UploadURL: url, ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}

// URL 访问地址。
func (m *Module) URL(ctx context.Context, fileID int64, spec string) (string, error) {
	var bucket, key string
	if err := m.pool.QueryRow(ctx, `SELECT bucket, object_key FROM med_file WHERE id=$1`, fileID).Scan(&bucket, &key); err != nil {
		return "", err
	}
	return m.storage.URL(ctx, bucket, key, spec)
}

// Bind 引用计数 +1。
func (m *Module) Bind(ctx context.Context, fileID int64, ownerRef string) error {
	_, err := m.pool.Exec(ctx,
		`UPDATE med_file SET ref_count=ref_count+1, owner_ref=$2 WHERE id=$1`, fileID, ownerRef)
	return err
}

// Unbind 引用计数 -1，归零置孤儿待清理。
func (m *Module) Unbind(ctx context.Context, fileID int64, _ string) error {
	_, err := m.pool.Exec(ctx,
		`UPDATE med_file SET ref_count=ref_count-1,
			status=CASE WHEN ref_count-1<=0 THEN 0 ELSE status END
		 WHERE id=$1 AND ref_count>0`, fileID)
	return err
}

// handleUpload 申请预签名直传（契约：JSON body {filename, content_type, size}）。
func (m *Module) handleUpload(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil ||
		body.Filename == "" || body.ContentType == "" || body.Size <= 0 {
		writeMediaErr(w, http.StatusBadRequest, 1, "参数错误")
		return
	}
	ticket, err := m.Upload(req.Context(), body.ContentType, body.Size, "user")
	if err != nil {
		writeMediaErr(w, http.StatusBadRequest, 1, err.Error())
		return
	}
	writeMediaData(w, map[string]any{
		"file_id":    strconv.FormatInt(ticket.FileID, 10),
		"upload_url": ticket.UploadURL,
		"expires_in": int(time.Until(ticket.ExpiresAt).Seconds()),
	})
}

func writeMediaData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

func writeMediaErr(w http.ResponseWriter, status, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}

var _ contract.Facade = (*Module)(nil)
