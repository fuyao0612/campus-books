package payments

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"campus-books/config"
	"github.com/smartwalle/alipay/v3"
)

// 生成的密钥仅供测试，绝不读取或复用用户的真实应用密钥。
func testClient(t *testing.T) (*alipay.Client, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	appKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(appKey)})
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.AlipayConfig{Sandbox: true, AppID: "test-app", AppPrivateKey: string(private), AlipayPublicKey: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))}
	client, err := NewAlipay(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return client, key
}
func signBytes(t *testing.T, key *rsa.PrivateKey, data []byte) string {
	t.Helper()
	hash := sha256.Sum256(data)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

// 保留 SDK 原始请求编码和响应验签，仅将传输目标替换为本机 HTTP 服务。
type redirectedTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (r redirectedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = r.target.Scheme, r.target.Host
	copy.URL = &u
	copy.Host = r.target.Host
	return r.base.RoundTrip(copy)
}
func TestSDKGatewaySignedHTTP(t *testing.T) {
	client, key := testClient(t)
	mode := "paid"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("app_id") != "test-app" || r.Form.Get("sign") == "" {
			t.Error("SDK没有签名请求")
		}
		method := r.Form.Get("method")
		var biz struct {
			Number string `json:"out_trade_no"`
		}
		_ = json.Unmarshal([]byte(r.Form.Get("biz_content")), &biz)
		if biz.Number != "order-test" {
			t.Error("错误的订单号")
		}
		body := `{"code":"10000","out_trade_no":"order-test","trade_no":"trade-1","trade_status":"TRADE_SUCCESS","total_amount":"19.99","send_pay_date":"2026-11-16 12:00:00"}`
		if method == "alipay.trade.close" {
			body = `{"code":"10000","out_trade_no":"order-test","trade_no":"trade-1"}`
		}
		if mode == "missing" {
			body = `{"code":"40004","sub_code":"ACQ.TRADE_NOT_EXIST"}`
		}
		if mode == "failed" {
			body = `{"code":"40004","sub_code":"ACQ.ACCESS_FORBIDDEN"}`
		}
		sig := signBytes(t, key, []byte(body))
		if mode == "bad-sign" {
			body = strings.Replace(body, "19.99", "29.99", 1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"`+strings.ReplaceAll(method, ".", "_")+`_response":`+body+`,"sign":"`+sig+`"}`)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client.Client = &http.Client{Transport: redirectedTransport{target, http.DefaultTransport}}
	g := SDKGateway{Client: client}
	trade, err := g.Query(context.Background(), "order-test")
	if err != nil || trade.Amount != 1999 || trade.TradeNo != "trade-1" {
		t.Fatalf("查单失败: %+v %v", trade, err)
	}
	if err := g.Close(context.Background(), "order-test"); err != nil {
		t.Fatal(err)
	}
	mode = "missing"
	if _, err := g.Query(context.Background(), "order-test"); !errors.Is(err, ErrTradeMissing) {
		t.Fatal(err)
	}
	mode = "failed"
	if _, err := g.Query(context.Background(), "order-test"); !errors.Is(err, ErrGateway) {
		t.Fatal(err)
	}
	mode = "bad-sign"
	if _, err := g.Query(context.Background(), "order-test"); err == nil {
		t.Fatal("被篡改的响应竟然通过验签")
	}
	if calls != 5 {
		t.Fatalf("实际HTTP次数 %d", calls)
	}
}
func TestSignedPagePay(t *testing.T) {
	client, _ := testClient(t)
	p := alipay.TradePagePay{Trade: alipay.Trade{Subject: "测试教材", OutTradeNo: "order-test", TotalAmount: "19.99", ProductCode: "FAST_INSTANT_TRADE_PAY", NotifyURL: "https://callback.example/api/payments/alipay/notify", TimeExpire: "2026-11-16 12:15:00"}}
	u, err := client.TradePagePay(p)
	if err != nil {
		t.Fatal(err)
	}
	if client.Production() || !strings.Contains(u.Host, "sandbox") || u.Query().Get("sign") == "" {
		t.Fatalf("环境或签名错误")
	}
	var biz map[string]interface{}
	_ = json.Unmarshal([]byte(u.Query().Get("biz_content")), &biz)
	if biz["total_amount"] != "19.99" || biz["out_trade_no"] != "order-test" || biz["time_expire"] != "2026-11-16 12:15:00" {
		t.Fatal(biz)
	}
}
func TestValidateNotifyURL(t *testing.T) {
	for _, address := range []string{"", "http://public.example/api/payments/alipay/notify", "https://localhost/api/payments/alipay/notify", "https://127.0.0.1/api/payments/alipay/notify", "https://10.0.0.1/api/payments/alipay/notify", "https://public.example/wrong", "https://user:pass@public.example/api/payments/alipay/notify"} {
		if ValidateNotifyURL(address) == nil {
			t.Errorf("不应接受 %s", address)
		}
	}
	if err := ValidateNotifyURL("https://public.example/api/payments/alipay/notify"); err != nil {
		t.Fatal(err)
	}
}

func TestRejectApplicationPublicKeyAsAlipayKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.AlipayConfig{Sandbox: true, AppID: "test", AppPrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), AlipayPublicKey: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))}
	if _, err := NewAlipay(cfg); err == nil {
		t.Fatal("误填应用公钥应在初始化时被识别")
	}
}
