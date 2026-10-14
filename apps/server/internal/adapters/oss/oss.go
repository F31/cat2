// Package oss 对象存储适配器骨架（云 OSS）。
// 一期实现预签名直传与访问 URL；具体供应商签名算法在接入时补全。
package oss

import (
	"context"
	"fmt"
	"io"
	"time"

	"catcube/internal/ports"
)

// Client OSS 客户端。
type Client struct {
	Endpoint string
	Bucket   string
	CDNHost  string
	// AccessKey/SecretKey 经启动配置注入，不入日志。
	accessKey string
	secretKey string
}

func NewClient(endpoint, bucket, cdnHost, ak, sk string) *Client {
	return &Client{Endpoint: endpoint, Bucket: bucket, CDNHost: cdnHost, accessKey: ak, secretKey: sk}
}

func (c *Client) PresignUpload(_ context.Context, bucket, objectKey, mime string, ttl time.Duration) (string, error) {
	if c.Endpoint == "" {
		return "", fmt.Errorf("oss: 未配置 endpoint")
	}
	// TODO(供应商接入): 按云厂商签名 V4 生成预签名 PUT URL
	return fmt.Sprintf("%s/%s/%s?expires=%d&mime=%s", c.Endpoint, bucket, objectKey, int64(ttl.Seconds()), mime), nil
}

func (c *Client) URL(_ context.Context, bucket, objectKey, spec string) (string, error) {
	host := c.CDNHost
	if host == "" {
		host = c.Endpoint
	}
	u := fmt.Sprintf("%s/%s/%s", host, bucket, objectKey)
	if spec != "" {
		u += "?x-oss-process=" + spec
	}
	return u, nil
}

func (c *Client) Put(_ context.Context, _, _ string, _ io.Reader) error {
	return fmt.Errorf("oss: Put 待供应商接入（一期走预签名直传）")
}

func (c *Client) Delete(_ context.Context, _, _ string) error {
	return fmt.Errorf("oss: Delete 待供应商接入")
}

var _ ports.Storage = (*Client)(nil)
