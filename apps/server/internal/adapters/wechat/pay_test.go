package wechat

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"catcube/internal/ports"
)

const testAPIv3Key = "0123456789abcdef0123456789abcdef"

type payFixture struct {
	key      *rsa.PrivateKey
	certPEM  string
	apiKey   string
	now      time.Time
	notify   string
	appID    string
	mchID    string
	serialNo string
}

func newPayFixture(t *testing.T) *payFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(4242),
		Subject:      pkix.Name{CommonName: "wechatpay-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &payFixture{
		key:      key,
		certPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		apiKey:   testAPIv3Key,
		now:      time.Now(),
		notify:   "https://example.com/api/mp/v1/pay/notify",
		appID:    "wx-test-app",
		mchID:    "1900000001",
		serialNo: "SERIAL-1",
	}
}

func (f *payFixture) client(t *testing.T, baseURL string) *PayClient {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(f.key)
	if err != nil {
		t.Fatal(err)
	}
	pc, err := NewPayClient(PayConfig{
		AppID: f.appID, MchID: f.mchID, SerialNo: f.serialNo,
		PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		APIv3Key:   f.apiKey, NotifyURL: f.notify, PlatformCert: f.certPEM, BaseURL: baseURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	pc.now = func() time.Time { return f.now }
	return pc
}

func rsaSign(t *testing.T, key *rsa.PrivateKey, msg string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

func parseAuth(t *testing.T, header string) (ts, nonce, sig string) {
	t.Helper()
	if !strings.HasPrefix(header, "WECHATPAY2-SHA256-RSA2048 ") {
		t.Fatalf("Authorization 前缀错误: %s", header)
	}
	for _, part := range strings.Split(strings.TrimPrefix(header, "WECHATPAY2-SHA256-RSA2048 "), ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v := strings.Trim(kv[1], `"`)
		switch kv[0] {
		case "timestamp":
			ts = v
		case "nonce_str":
			nonce = v
		case "signature":
			sig = v
		}
	}
	if ts == "" || nonce == "" || sig == "" {
		t.Fatalf("Authorization 缺字段: %s", header)
	}
	return
}

func verifyRSASignature(t *testing.T, pub *rsa.PublicKey, msg, sigB64 string) {
	t.Helper()
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(msg))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("签名验签失败: %v", err)
	}
}

func TestPayPrepayJSAPI(t *testing.T) {
	f := newPayFixture(t)
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/pay/transactions/jsapi" {
			t.Fatalf("请求错误: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		ts, nonce, sig := parseAuth(t, r.Header.Get("Authorization"))
		verifyRSASignature(t, &f.key.PublicKey, "POST\n/v3/pay/transactions/jsapi\n"+ts+"\n"+nonce+"\n"+string(raw)+"\n", sig)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"prepay_id":"wx-prepay-1"}`))
	}))
	defer srv.Close()
	pc := f.client(t, srv.URL)

	res, err := pc.Prepay(context.Background(), ports.PayRequest{
		OutTradeNo: "CC1001", AmountFen: 990, Subject: "会员", OpenID: "openid-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.PrepayID != "wx-prepay-1" || res.Package != "prepay_id=wx-prepay-1" || res.SignType != "RSA" || res.PaySign == "" {
		t.Fatalf("预支付参数异常: %+v", res)
	}
	if gotBody["appid"] != f.appID || gotBody["mchid"] != f.mchID || gotBody["notify_url"] != f.notify {
		t.Fatalf("下单参数错误: %+v", gotBody)
	}
	if gotBody["out_trade_no"] != "CC1001" {
		t.Fatalf("商户单号错误: %+v", gotBody)
	}
	amount := gotBody["amount"].(map[string]any)
	if int64(amount["total"].(float64)) != 990 {
		t.Fatalf("金额错误: %+v", amount)
	}
	payer := gotBody["payer"].(map[string]any)
	if payer["openid"] != "openid-1" {
		t.Fatalf("openid 错误: %+v", payer)
	}
	verifyRSASignature(t, &f.key.PublicKey,
		f.appID+"\n"+res.TimeStamp+"\n"+res.NonceStr+"\n"+res.Package+"\n", res.PaySign)
}

func TestPayPrepayRequiresOpenID(t *testing.T) {
	f := newPayFixture(t)
	pc := f.client(t, "http://127.0.0.1:1")
	if _, err := pc.Prepay(context.Background(), ports.PayRequest{OutTradeNo: "X", AmountFen: 1}); err == nil {
		t.Fatal("缺少 openid 应失败")
	}
}

func TestPayNotifyVerifyAndDecrypt(t *testing.T) {
	f := newPayFixture(t)
	pc := f.client(t, "http://127.0.0.1:1")

	plain := []byte(`{"out_trade_no":"CC1001","trade_state":"SUCCESS","amount":{"total":990}}`)
	block, err := aes.NewCipher([]byte(f.apiKey))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := "0123456789ab"
	ciphertext := gcm.Seal(nil, []byte(nonce), plain, []byte("transaction"))
	body, _ := json.Marshal(map[string]any{
		"event_type": "TRANSACTION.SUCCESS",
		"resource": map[string]string{
			"algorithm": "AEAD_AES_256_GCM", "ciphertext": base64.StdEncoding.EncodeToString(ciphertext),
			"associated_data": "transaction", "nonce": nonce,
		},
	})
	ts := fmt.Sprintf("%d", f.now.Unix())
	headerNonce := "notify-nonce"
	headers := map[string]string{
		"Wechatpay-Timestamp": ts,
		"Wechatpay-Nonce":     headerNonce,
		"Wechatpay-Signature": rsaSign(t, f.key, ts+"\n"+headerNonce+"\n"+string(body)+"\n"),
		"Wechatpay-Serial":    "SERIAL-1",
	}
	out, amount, err := pc.VerifyNotify(context.Background(), headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if out != "CC1001" || amount != 990 {
		t.Fatalf("解密结果错误: %s %d", out, amount)
	}

	headers["Wechatpay-Signature"] = base64.StdEncoding.EncodeToString([]byte("bad"))
	if _, _, err := pc.VerifyNotify(context.Background(), headers, body); err == nil {
		t.Fatal("错误签名应失败")
	}
	headers["Wechatpay-Signature"] = rsaSign(t, f.key, ts+"\n"+headerNonce+"\n"+string(body)+"\n")
	if _, _, err := pc.VerifyNotify(context.Background(), headers, append(body, ' ')); err == nil {
		t.Fatal("篡改报文应失败")
	}
}

func TestPayNotifyRejectsExpiredTimestamp(t *testing.T) {
	f := newPayFixture(t)
	pc := f.client(t, "http://127.0.0.1:1")
	body := []byte(`{}`)
	ts := fmt.Sprintf("%d", f.now.Add(-10*time.Minute).Unix())
	headers := map[string]string{
		"Wechatpay-Timestamp": ts, "Wechatpay-Nonce": "n",
		"Wechatpay-Signature": rsaSign(t, f.key, ts+"\n"+"n"+"\n"+string(body)+"\n"),
	}
	if _, _, err := pc.VerifyNotify(context.Background(), headers, body); err == nil {
		t.Fatal("过期回调应失败")
	}
}

func TestPayRefund(t *testing.T) {
	f := newPayFixture(t)
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/refund/domestic/refunds" {
			t.Fatalf("退款路径错误: %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		ts, nonce, sig := parseAuth(t, r.Header.Get("Authorization"))
		verifyRSASignature(t, &f.key.PublicKey, "POST\n/v3/refund/domestic/refunds\n"+ts+"\n"+nonce+"\n"+string(raw)+"\n", sig)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	pc := f.client(t, srv.URL)
	if err := pc.Refund(context.Background(), "CC1001", "RF-1", 300, 990); err != nil {
		t.Fatal(err)
	}
	if got["out_trade_no"] != "CC1001" || got["out_refund_no"] != "RF-1" {
		t.Fatalf("退款参数错误: %+v", got)
	}
	amount := got["amount"].(map[string]any)
	if int64(amount["refund"].(float64)) != 300 || int64(amount["total"].(float64)) != 990 {
		t.Fatalf("退款金额错误: %+v", amount)
	}
	if err := pc.Refund(context.Background(), "CC1001", "RF-2", 1000, 990); err == nil {
		t.Fatal("超额退款应失败")
	}
}

func TestNewPayClientValidation(t *testing.T) {
	if _, err := NewPayClient(PayConfig{}); err == nil {
		t.Fatal("空配置应失败")
	}
}
