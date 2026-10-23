package oss

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func ossSig(secret, sts string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(sts))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestPresignUploadSignature(t *testing.T) {
	c := NewClient("https://oss.example.com", "bucket", "", "AK", "SK")
	fixed := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return fixed }
	u, err := c.PresignUpload(context.Background(), "bucket", "a/b.png", "image/png", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("OSSAccessKeyId") != "AK" || q.Get("Signature") == "" {
		t.Fatalf("预签名参数缺失: %s", u)
	}
	expires := fixed.Add(15 * time.Minute).Unix()
	if q.Get("Expires") != strconv.FormatInt(expires, 10) {
		t.Fatalf("Expires 错误: %s", q.Get("Expires"))
	}
	want := ossSig("SK", "PUT\n\nimage/png\n"+strconv.FormatInt(expires, 10)+"\n/bucket/a/b.png")
	if q.Get("Signature") != want {
		t.Fatalf("签名错误: got %s want %s", q.Get("Signature"), want)
	}
	if !strings.HasSuffix(parsed.Path, "/bucket/a/b.png") {
		t.Fatalf("对象路径错误: %s", parsed.Path)
	}
}

func TestPutDeleteSigned(t *testing.T) {
	fixed := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	date := fixed.UTC().Format(http.TimeFormat)
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			if string(body) != "hello" {
				t.Fatalf("body: %s", body)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "bucket", "", "AK", "SK")
	c.now = func() time.Time { return fixed }
	if err := c.Put(context.Background(), "bucket", "x/y.txt", strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(context.Background(), "bucket", "x/y.txt"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("请求数错误: %v", got)
	}
	putWant := "OSS AK:" + ossSig("SK", "PUT\n\n\n"+date+"\n/bucket/x/y.txt")
	delWant := "OSS AK:" + ossSig("SK", "DELETE\n\n\n"+date+"\n/bucket/x/y.txt")
	if !strings.Contains(got[0], putWant) || !strings.Contains(got[0], "PUT /bucket/x/y.txt") {
		t.Fatalf("PUT 签名错误: %s", got[0])
	}
	if !strings.Contains(got[1], delWant) || !strings.Contains(got[1], "DELETE /bucket/x/y.txt") {
		t.Fatalf("DELETE 签名错误: %s", got[1])
	}
}

func TestPutRequiresKeys(t *testing.T) {
	c := NewClient("https://oss.example.com", "bucket", "", "", "")
	if err := c.Put(context.Background(), "bucket", "x", strings.NewReader("x")); err == nil {
		t.Fatal("未配置密钥应失败")
	}
}

func TestURLSpec(t *testing.T) {
	c := NewClient("https://oss.example.com", "bucket", "https://cdn.example.com", "AK", "SK")
	u, err := c.URL(context.Background(), "bucket", "a/b.png", "image/resize,w_300")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "/bucket/a/b.png") || !strings.Contains(u, "x-oss-process=image%2Fresize%2Cw_300") {
		t.Fatalf("URL 错误: %s", u)
	}
}
