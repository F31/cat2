// Package media 共享业务域模块：统一上传、缩略图、引用计数与孤儿清理。
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/module"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/ports"
	"catcube/internal/shared/media/contract"
	"catcube/pkg/ids"
)

// 允许的文件类型与大小（PRD 8.9.2）。
var allowedMime = map[string]bool{"image/jpeg": true, "image/jpg": true, "image/png": true}

const maxSizeBytes = 5 << 20 // 5MB

// Module media 模块。
type Module struct {
	pool     *db.Pool
	storage  ports.Storage
	identity idcontract.Facade
}

func New(pool *db.Pool, storage ports.Storage, identity idcontract.Facade) *Module {
	return &Module{pool: pool, storage: storage, identity: identity}
}

func (m *Module) Name() string        { return "media" }
func (m *Module) Rank() module.Rank   { return module.RankShared }
func (m *Module) DependsOn() []string { return nil }

// Register 注册上传接口与孤儿清理任务。
func (m *Module) Register(r *module.Registry) error {
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/media/uploads", Handler: m.handleUpload},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/media/upload", Handler: m.auth(m.handleUserUpload)},
		// 后台：组织资质等材料上传与访问（运营管理）
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/media/uploads", Handler: m.handleAdminUpload, Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/media/upload", Handler: m.handleAdminMultipartUpload, Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/media/{id}", Handler: m.handleAdminURL, Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/media/{id}/raw", Handler: m.handleAdminRaw, Admin: true},
	)
	// 孤儿文件定时清理（引用计数归零且超保留期 → 删对象 + 删行）
	r.Jobs = append(r.Jobs, module.JobDef{Name: "media.purge_orphans", Interval: 6 * time.Hour, Run: m.PurgeOrphans})
	return nil
}

// PurgeOrphans 清理孤儿文件（status=0 且超 7 天保留期）：先删对象存储，再删元数据。
func (m *Module) PurgeOrphans(ctx context.Context) error {
	rows, err := m.pool.Query(ctx, `
		SELECT id, bucket, object_key FROM med_file
		WHERE status=0 AND created_at < now() - interval '7 days' ORDER BY created_at LIMIT 200`)
	if err != nil {
		return err
	}
	type orphan struct {
		id     int64
		bucket string
		key    string
	}
	var list []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.id, &o.bucket, &o.key); err != nil {
			rows.Close()
			return err
		}
		list = append(list, o)
	}
	rows.Close()
	for _, o := range list {
		if m.storage != nil {
			if err := m.storage.Delete(ctx, o.bucket, o.key); err != nil {
				continue // 对象删除失败保留元数据待下次重试
			}
		}
		if _, err := m.pool.Exec(ctx, `DELETE FROM med_blob WHERE file_id=$1`, o.id); err != nil {
			return err
		}
		if _, err := m.pool.Exec(ctx, `DELETE FROM med_file WHERE id=$1 AND status=0`, o.id); err != nil {
			return err
		}
	}
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

// URL 访问地址（spec 见 specs.go：缩略图/封面走 CDN 处理参数）。
func (m *Module) URL(ctx context.Context, fileID int64, spec string) (string, error) {
	sp, err := ResolveSpec(spec)
	if err != nil {
		return "", err
	}
	var bucket, key string
	if err := m.pool.QueryRow(ctx, `SELECT bucket, object_key FROM med_file WHERE id=$1`, fileID).Scan(&bucket, &key); err != nil {
		return "", err
	}
	return m.storage.URL(ctx, bucket, key, sp.ProviderOps)
}

// Bind 引用计数 +1（owner_ref 为 jsonb：以 {"ref":"type:id"} 结构存储，兼容 EntityRef）。
func (m *Module) Bind(ctx context.Context, fileID int64, ownerRef string) error {
	raw, err := json.Marshal(map[string]string{"ref": ownerRef})
	if err != nil {
		return err
	}
	_, err = m.pool.Exec(ctx,
		`UPDATE med_file SET ref_count=ref_count+1, owner_ref=$2::jsonb WHERE id=$1`, fileID, raw)
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

// UploadBytes 本地落库上传（无对象存储时的资质材料兜底）。
func (m *Module) UploadBytes(ctx context.Context, _ string, mime string, data []byte) (int64, error) {
	if !allowedMime[mime] {
		return 0, fmt.Errorf("media: 仅允许 JPG/JPEG/PNG")
	}
	if len(data) == 0 || int64(len(data)) > maxSizeBytes {
		return 0, fmt.Errorf("media: 文件大小不能超过 5MB")
	}
	id := ids.New()
	objectKey := fmt.Sprintf("org/%d", id)
	if _, err := m.pool.Exec(ctx,
		`INSERT INTO med_file (id, bucket, object_key, mime, size_bytes, ref_count, status, created_at)
		 VALUES ($1,'default',$2,$3,$4,0,1,now())`, int64(id), objectKey, mime, len(data)); err != nil {
		return 0, err
	}
	if _, err := m.pool.Exec(ctx, `INSERT INTO med_blob (file_id, data) VALUES ($1,$2)`, int64(id), data); err != nil {
		return 0, err
	}
	return int64(id), nil
}

// Raw 读取本地落库文件。
func (m *Module) Raw(ctx context.Context, fileID int64) (string, []byte, error) {
	var mime string
	var data []byte
	if err := m.pool.QueryRow(ctx, `
		SELECT f.mime, b.data FROM med_file f JOIN med_blob b ON b.file_id=f.id WHERE f.id=$1`, fileID).
		Scan(&mime, &data); err != nil {
		return "", nil, err
	}
	return mime, data, nil
}

// BlobExists 是否有本地落库文件。
func (m *Module) BlobExists(ctx context.Context, fileID int64) bool {
	var n int
	err := m.pool.QueryRow(ctx, `SELECT count(1) FROM med_blob WHERE file_id=$1`, fileID).Scan(&n)
	return err == nil && n > 0
}

func (m *Module) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		h := req.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			writeMediaErr(w, http.StatusUnauthorized, 2, "未登录")
			return
		}
		if m.identity == nil {
			writeMediaErr(w, http.StatusUnauthorized, 2, "未登录")
			return
		}
		if _, _, err := m.identity.Authenticate(req.Context(), strings.TrimPrefix(h, "Bearer ")); err != nil {
			writeMediaErr(w, http.StatusUnauthorized, 2, "登录已失效")
			return
		}
		next(w, req)
	}
}

func (m *Module) handleUserUpload(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseMultipartForm(maxSizeBytes + (1 << 20)); err != nil {
		writeMediaErr(w, http.StatusBadRequest, 1, "上传参数错误")
		return
	}
	file, header, err := req.FormFile("file")
	if err != nil {
		writeMediaErr(w, http.StatusBadRequest, 1, "缺少文件")
		return
	}
	defer file.Close()
	mime := header.Header.Get("Content-Type")
	data, err := io.ReadAll(io.LimitReader(file, maxSizeBytes+1))
	if err != nil || int64(len(data)) > maxSizeBytes {
		writeMediaErr(w, http.StatusBadRequest, 1, "文件大小不能超过 5MB")
		return
	}
	id, err := m.UploadBytes(req.Context(), header.Filename, mime, data)
	if err != nil {
		writeMediaErr(w, http.StatusBadRequest, 1, err.Error())
		return
	}
	writeMediaData(w, map[string]any{"file_id": strconv.FormatInt(id, 10)})
}

func (m *Module) handleAdminMultipartUpload(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseMultipartForm(maxSizeBytes + (1 << 20)); err != nil {
		writeMediaErr(w, http.StatusBadRequest, 1, "上传参数错误")
		return
	}
	file, header, err := req.FormFile("file")
	if err != nil {
		writeMediaErr(w, http.StatusBadRequest, 1, "缺少文件")
		return
	}
	defer file.Close()
	mime := header.Header.Get("Content-Type")
	data, err := io.ReadAll(io.LimitReader(file, maxSizeBytes+1))
	if err != nil || int64(len(data)) > maxSizeBytes {
		writeMediaErr(w, http.StatusBadRequest, 1, "文件大小不能超过 5MB")
		return
	}
	id, err := m.UploadBytes(req.Context(), header.Filename, mime, data)
	if err != nil {
		writeMediaErr(w, http.StatusBadRequest, 1, err.Error())
		return
	}
	writeMediaData(w, map[string]any{"file_id": strconv.FormatInt(id, 10)})
}

func (m *Module) handleAdminRaw(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeMediaErr(w, http.StatusBadRequest, 1, "参数错误")
		return
	}
	mime, data, err := m.Raw(req.Context(), id)
	if err != nil {
		writeMediaErr(w, http.StatusNotFound, 9, "文件不存在")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = io.Copy(w, bytes.NewReader(data))
}

func (m *Module) handleAdminUpload(w http.ResponseWriter, req *http.Request) {
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
	ticket, err := m.Upload(req.Context(), body.ContentType, body.Size, "org")
	if err != nil {
		writeMediaErr(w, http.StatusBadRequest, 1, err.Error())
		return
	}
	writeMediaData(w, map[string]any{
		"file_id": strconv.FormatInt(ticket.FileID, 10), "upload_url": ticket.UploadURL,
		"expires_in": int(time.Until(ticket.ExpiresAt).Seconds()),
	})
}

func (m *Module) handleAdminURL(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeMediaErr(w, http.StatusBadRequest, 1, "参数错误")
		return
	}
	if m.BlobExists(req.Context(), id) {
		writeMediaData(w, map[string]any{"url": fmt.Sprintf("/api/admin/v1/media/%d/raw", id)})
		return
	}
	url, err := m.URL(req.Context(), id, "orig")
	if err != nil {
		writeMediaErr(w, http.StatusNotFound, 9, "文件不存在")
		return
	}
	writeMediaData(w, map[string]any{"url": url})
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
