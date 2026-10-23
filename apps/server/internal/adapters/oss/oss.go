// Package oss 对象存储适配器：阿里云 OSS V1 签名（路径风格），
// 支持预签名直传、签名 Put/Delete 与访问 URL；未配置 endpoint 时回退本地文件系统（仅开发）。
package oss

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"catcube/internal/ports"
)

// Client OSS 客户端。
type Client struct {
	Endpoint string
	Bucket   string
	CDNHost  string
	HTTP     *http.Client
	// AccessKey/SecretKey 经启动配置注入，不入日志。
	accessKey string
	secretKey string
	now       func() time.Time
}

func NewClient(endpoint, bucket, cdnHost, ak, sk string) *Client {
	return &Client{
		Endpoint: endpoint, Bucket: bucket, CDNHost: cdnHost,
		accessKey: ak, secretKey: sk,
		HTTP: &http.Client{Timeout: 10 * time.Second},
		now:  time.Now,
	}
}

// objectURL 生成路径风格对象地址（endpoint/bucket/key），endpoint 必须为绝对 URL。
func (c *Client) objectURL(bucket, objectKey string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(c.Endpoint, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("oss: endpoint 必须是绝对 URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + bucket + "/" + strings.TrimLeft(objectKey, "/")
	return u, nil
}

func (c *Client) sign(method, bucket, objectKey string, expires int64, contentType string) string {
	sts := method + "\n\n" + contentType + "\n" + strconv.FormatInt(expires, 10) + "\n/" + bucket + "/" + strings.TrimLeft(objectKey, "/")
	mac := hmac.New(sha1.New, []byte(c.secretKey))
	_, _ = mac.Write([]byte(sts))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (c *Client) requireKeys() error {
	if c.accessKey == "" || c.secretKey == "" {
		return fmt.Errorf("oss: 未配置 AccessKey/SecretKey")
	}
	return nil
}

// PresignUpload 生成 PUT 直传预签名 URL（15 分钟有效；Content-Type 必须与签名一致）。
func (c *Client) PresignUpload(_ context.Context, bucket, objectKey, mime string, ttl time.Duration) (string, error) {
	if c.Endpoint == "" {
		// 未配置对象存储（开发/联调）：返回占位直传地址，接口契约可用（真实上传走云厂商）。
		return fmt.Sprintf("http://127.0.0.1:9000/%s/%s?dev=1", bucket, objectKey), nil
	}
	if err := c.requireKeys(); err != nil {
		return "", err
	}
	u, err := c.objectURL(bucket, objectKey)
	if err != nil {
		return "", err
	}
	expires := c.now().Add(ttl).Unix()
	sig := c.sign(http.MethodPut, bucket, objectKey, expires, mime)
	q := u.Query()
	q.Set("OSSAccessKeyId", c.accessKey)
	q.Set("Expires", strconv.FormatInt(expires, 10))
	q.Set("Signature", sig)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// URL 访问地址（spec 形如 image/resize,w_300；走 CDN/对象域名）。
func (c *Client) URL(_ context.Context, bucket, objectKey, spec string) (string, error) {
	host := strings.TrimRight(c.CDNHost, "/")
	if host == "" {
		host = strings.TrimRight(c.Endpoint, "/")
	}
	if host == "" {
		// 开发态：本地文件系统回退（与服务端 Put 落盘位置一致）。
		return localPath(bucket, objectKey), nil
	}
	u, err := url.Parse(host)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("oss: 访问域名非法")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + bucket + "/" + strings.TrimLeft(objectKey, "/")
	if spec != "" {
		q := u.Query()
		q.Set("x-oss-process", spec)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// localPath 开发态本地对象存储路径（未配置 endpoint 时启用）。
func localPath(bucket, objectKey string) string {
	base := os.Getenv("CATCUBE_OSS_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "catcube-oss")
	}
	return filepath.Join(base, bucket, filepath.FromSlash(objectKey))
}

// doSigned 发送签名请求（Date + Authorization），用于服务端 Put/Delete。
func (c *Client) doSigned(ctx context.Context, method, bucket, objectKey string, body io.Reader) error {
	if err := c.requireKeys(); err != nil {
		return err
	}
	u, err := c.objectURL(bucket, objectKey)
	if err != nil {
		return err
	}
	date := c.now().UTC().Format(http.TimeFormat)
	// 服务端签名使用 Date，不是 Expires。
	sts := method + "\n\n\n" + date + "\n/" + bucket + "/" + strings.TrimLeft(objectKey, "/")
	mac := hmac.New(sha1.New, []byte(c.secretKey))
	_, _ = mac.Write([]byte(sts))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Date", date)
	req.Header.Set("Authorization", "OSS "+c.accessKey+":"+sig)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("oss: %s %s 失败 HTTP %d: %s", method, objectKey, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (c *Client) Put(ctx context.Context, bucket, objectKey string, r io.Reader) error {
	if c.Endpoint == "" {
		path := localPath(bucket, objectKey)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, r)
		return err
	}
	return c.doSigned(ctx, http.MethodPut, bucket, objectKey, r)
}

func (c *Client) Delete(ctx context.Context, bucket, objectKey string) error {
	if c.Endpoint == "" {
		err := os.Remove(localPath(bucket, objectKey))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return c.doSigned(ctx, http.MethodDelete, bucket, objectKey, nil)
}

var _ ports.Storage = (*Client)(nil)
