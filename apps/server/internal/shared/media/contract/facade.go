// Package contract media 对外契约：上传、URL、绑定与引用计数。
package contract

import (
	"context"
	"time"
)

// File 媒体文件。
type File struct {
	ID        int64  `json:"id,string"`
	Bucket    string `json:"bucket"`
	ObjectKey string `json:"object_key"`
	Mime      string `json:"mime"`
	SizeBytes int64  `json:"size_bytes"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	RefCount  int    `json:"ref_count"`
}

// UploadTicket 预签名直传凭证。
type UploadTicket struct {
	FileID    int64     `json:"file_id,string"`
	UploadURL string    `json:"upload_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Facade media 对外接口。
type Facade interface {
	// Upload 申请预签名直传（JPG/JPEG/PNG ≤5MB；封面 16:9 最小 750×422）。
	Upload(ctx context.Context, mime string, sizeBytes int64, purpose string) (UploadTicket, error)
	// URL 访问地址（spec 如缩略图规格）。
	URL(ctx context.Context, fileID int64, spec string) (string, error)
	// Bind 记录引用（引用计数+1）。
	Bind(ctx context.Context, fileID int64, ownerRef string) error
	// Unbind 解除引用（归零进孤儿清理队列）。
	Unbind(ctx context.Context, fileID int64, ownerRef string) error
}
