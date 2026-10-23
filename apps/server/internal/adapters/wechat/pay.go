package wechat

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"catcube/internal/ports"
)

const defaultPayBaseURL = "https://api.mch.weixin.qq.com"

// PayConfig 微信支付 V3 商户配置。私钥/平台证书传 PEM 内容（装配层可从文件读取）。
type PayConfig struct {
	AppID        string
	MchID        string
	SerialNo     string
	PrivateKey   string
	APIv3Key     string
	NotifyURL    string
	PlatformCert string
	BaseURL      string // 测试注入；空=微信生产网关
}

// PayClient 微信支付 V3（JSAPI 下单 / 回调验签解密 / 退款）。
type PayClient struct {
	appID     string
	mchID     string
	serialNo  string
	apiV3Key  []byte
	notifyURL string
	baseURL   string
	http      *http.Client
	key       *rsa.PrivateKey
	certs     []*x509.Certificate
	now       func() time.Time
}

// NewPayClient 构造支付客户端；配置不完整或密钥/证书非法时拒绝启动，避免静默回退 DevClient。
func NewPayClient(cfg PayConfig) (*PayClient, error) {
	cfg.AppID = strings.TrimSpace(cfg.AppID)
	cfg.MchID = strings.TrimSpace(cfg.MchID)
	cfg.SerialNo = strings.TrimSpace(cfg.SerialNo)
	cfg.NotifyURL = strings.TrimSpace(cfg.NotifyURL)
	if cfg.AppID == "" || cfg.MchID == "" || cfg.SerialNo == "" || cfg.NotifyURL == "" {
		return nil, errors.New("wechat pay: appid/mchid/serial_no/notify_url 必填")
	}
	u, err := url.ParseRequestURI(cfg.NotifyURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("wechat pay: notify_url 必须是绝对 HTTP(S) URL")
	}
	if len(cfg.APIv3Key) != 32 {
		return nil, errors.New("wechat pay: APIv3Key 必须为 32 字节")
	}
	key, err := parseRSAPrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, err
	}
	certs, err := parseCertificates(cfg.PlatformCert)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultPayBaseURL
	}
	return &PayClient{
		appID: cfg.AppID, mchID: cfg.MchID, serialNo: cfg.SerialNo,
		apiV3Key: []byte(cfg.APIv3Key), notifyURL: cfg.NotifyURL, baseURL: baseURL,
		http: &http.Client{Timeout: 10 * time.Second}, key: key, certs: certs, now: time.Now,
	}, nil
}

func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil {
		return nil, errors.New("wechat pay: 商户私钥不是有效 PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, errors.New("wechat pay: 商户私钥不是 RSA")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	return nil, errors.New("wechat pay: 商户私钥解析失败")
}

func parseCertificates(raw string) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := []byte(strings.TrimSpace(raw))
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("wechat pay: 平台证书解析失败: %w", err)
			}
			certs = append(certs, cert)
		}
		rest = next
	}
	if len(certs) == 0 {
		return nil, errors.New("wechat pay: 平台证书为空")
	}
	return certs, nil
}

func (p *PayClient) randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (p *PayClient) sign(message string) (string, error) {
	sum := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// doV3 发起 APIv3 请求：商户私钥签名 + 状态码检查。
func (p *PayClient) doV3(ctx context.Context, method, path string, body any) ([]byte, error) {
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	nonce, err := p.randomNonce()
	if err != nil {
		return nil, err
	}
	ts := strconv.FormatInt(p.now().Unix(), 10)
	message := method + "\n" + path + "\n" + ts + "\n" + nonce + "\n" + string(raw) + "\n"
	signature, err := p.sign(message)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf(
		`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",signature="%s",timestamp="%s",serial_no="%s"`,
		p.mchID, nonce, signature, ts, p.serialNo))
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e apiError
		_ = json.Unmarshal(payload, &e)
		if e.Message == "" {
			e.Message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("wechat pay: %s %s (HTTP %d)", e.Code, e.Message, resp.StatusCode)
	}
	return payload, nil
}

// Prepay JSAPI 下单并生成小程序 requestPayment 参数。
func (p *PayClient) Prepay(ctx context.Context, req ports.PayRequest) (ports.PayPrepay, error) {
	if strings.TrimSpace(req.OpenID) == "" {
		return ports.PayPrepay{}, errors.New("wechat pay: 缺少 payer.openid")
	}
	if req.AmountFen <= 0 {
		return ports.PayPrepay{}, errors.New("wechat pay: 金额必须大于 0")
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		subject = "猫平方订单"
	}
	payload, err := p.doV3(ctx, http.MethodPost, "/v3/pay/transactions/jsapi", map[string]any{
		"appid": p.appID, "mchid": p.mchID, "description": subject,
		"out_trade_no": req.OutTradeNo, "notify_url": p.notifyURL,
		"amount": map[string]any{"total": req.AmountFen, "currency": "CNY"},
		"payer":  map[string]string{"openid": req.OpenID},
	})
	if err != nil {
		return ports.PayPrepay{}, err
	}
	var r struct {
		PrepayID string `json:"prepay_id"`
	}
	if err := json.Unmarshal(payload, &r); err != nil {
		return ports.PayPrepay{}, err
	}
	if r.PrepayID == "" {
		return ports.PayPrepay{}, errors.New("wechat pay: 下单响应缺少 prepay_id")
	}
	ts := strconv.FormatInt(p.now().Unix(), 10)
	nonce, err := p.randomNonce()
	if err != nil {
		return ports.PayPrepay{}, err
	}
	pkg := "prepay_id=" + r.PrepayID
	signature, err := p.sign(p.appID + "\n" + ts + "\n" + nonce + "\n" + pkg + "\n")
	if err != nil {
		return ports.PayPrepay{}, err
	}
	return ports.PayPrepay{
		PrepayID: r.PrepayID, TimeStamp: ts, NonceStr: nonce,
		Package: pkg, SignType: "RSA", PaySign: signature,
	}, nil
}

type notifyEnvelope struct {
	EventType string `json:"event_type"`
	Resource  struct {
		Algorithm      string `json:"algorithm"`
		Ciphertext     string `json:"ciphertext"`
		AssociatedData string `json:"associated_data"`
		Nonce          string `json:"nonce"`
	} `json:"resource"`
}

type transactionResource struct {
	OutTradeNo string `json:"out_trade_no"`
	TradeState string `json:"trade_state"`
	Amount     struct {
		Total int64 `json:"total"`
	} `json:"amount"`
}

// VerifyNotify 校验 Wechatpay-Signature 并用 APIv3Key 解密 resource；仅接受支付成功通知。
func (p *PayClient) VerifyNotify(ctx context.Context, headers map[string]string, body []byte) (string, int64, error) {
	ts := headers["Wechatpay-Timestamp"]
	nonce := headers["Wechatpay-Nonce"]
	sigB64 := headers["Wechatpay-Signature"]
	if ts == "" || nonce == "" || sigB64 == "" {
		return "", 0, errors.New("wechat pay: 回调签名头缺失")
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return "", 0, errors.New("wechat pay: 回调时间戳非法")
	}
	if d := p.now().Sub(time.Unix(sec, 0)); d > 5*time.Minute || d < -5*time.Minute {
		return "", 0, errors.New("wechat pay: 回调时间戳超出窗口")
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return "", 0, errors.New("wechat pay: 回调签名非法")
	}
	message := ts + "\n" + nonce + "\n" + string(body) + "\n"
	sum := sha256.Sum256([]byte(message))
	verified := false
	for _, cert := range p.certs {
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			continue
		}
		if rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) == nil {
			verified = true
			break
		}
	}
	if !verified {
		return "", 0, errors.New("wechat pay: 回调验签失败")
	}
	var env notifyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", 0, fmt.Errorf("wechat pay: 回调报文非法: %w", err)
	}
	if env.EventType != "TRANSACTION.SUCCESS" {
		return "", 0, fmt.Errorf("wechat pay: 非支付成功通知 %s", env.EventType)
	}
	if env.Resource.Algorithm != "AEAD_AES_256_GCM" {
		return "", 0, fmt.Errorf("wechat pay: 不支持的加密算法 %s", env.Resource.Algorithm)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Resource.Ciphertext)
	if err != nil {
		return "", 0, errors.New("wechat pay: 密文非法")
	}
	block, err := aes.NewCipher(p.apiV3Key)
	if err != nil {
		return "", 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", 0, err
	}
	plain, err := gcm.Open(nil, []byte(env.Resource.Nonce), ciphertext, []byte(env.Resource.AssociatedData))
	if err != nil {
		return "", 0, errors.New("wechat pay: 回调解密失败")
	}
	var tx transactionResource
	if err := json.Unmarshal(plain, &tx); err != nil {
		return "", 0, fmt.Errorf("wechat pay: 交易报文非法: %w", err)
	}
	if tx.OutTradeNo == "" || tx.Amount.Total <= 0 {
		return "", 0, errors.New("wechat pay: 交易报文缺少单号或金额")
	}
	if tx.TradeState != "" && tx.TradeState != "SUCCESS" {
		return "", 0, fmt.Errorf("wechat pay: 交易未成功 %s", tx.TradeState)
	}
	return tx.OutTradeNo, tx.Amount.Total, nil
}

// Refund 申请原路退款（受理即成功；最终结果以微信退款通知/查询为准）。
func (p *PayClient) Refund(ctx context.Context, outTradeNo, refundNo string, amountFen, totalFen int64) error {
	if outTradeNo == "" || refundNo == "" {
		return errors.New("wechat pay: 退款单号为空")
	}
	if amountFen <= 0 || totalFen <= 0 || amountFen > totalFen {
		return errors.New("wechat pay: 退款金额非法")
	}
	_, err := p.doV3(ctx, http.MethodPost, "/v3/refund/domestic/refunds", map[string]any{
		"out_trade_no": outTradeNo, "out_refund_no": refundNo,
		"amount": map[string]any{"refund": amountFen, "total": totalFen, "currency": "CNY"},
	})
	return err
}

var _ ports.Pay = (*PayClient)(nil)
