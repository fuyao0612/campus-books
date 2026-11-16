package handlers

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"campus-books/config"
	"campus-books/payments"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func notifyFixture(t *testing.T) (*PaymentHandler, *rsa.PrivateKey, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	appKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.AlipayConfig{Sandbox: true, AppID: "test-app", SellerID: "test-seller", AppPrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(appKey)})), AlipayPublicKey: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))}
	client, err := payments.NewAlipay(cfg)
	if err != nil {
		t.Fatal(err)
	}
	conn, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlserver.New(sqlserver.Config{Conn: conn}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = conn.Close()
	})
	return &PaymentHandler{DB: db, Config: cfg, Client: client, Service: &payments.Service{DB: db}}, key, m
}
func signedValues(t *testing.T, key *rsa.PrivateKey, changes map[string]string) url.Values {
	t.Helper()
	v := url.Values{"app_id": {"test-app"}, "seller_id": {"test-seller"}, "notify_type": {"trade_status_sync"}, "trade_status": {"TRADE_SUCCESS"}, "out_trade_no": {"order-test"}, "trade_no": {"trade-1"}, "total_amount": {"19.99"}, "gmt_payment": {"2026-11-16 12:05:00"}, "sign_type": {"RSA2"}}
	for k, val := range changes {
		v.Set(k, val)
	}
	keys := []string{}
	for k := range v {
		if k != "sign" && k != "sign_type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		parts = append(parts, k+"="+v.Get(k))
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "&")))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	v.Set("sign", base64.StdEncoding.EncodeToString(sig))
	return v
}
func notifyRequest(h *PaymentHandler, body, contentType string) *httptest.ResponseRecorder {
	r := gin.New()
	r.POST("/notify", h.Notify)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)
	return w
}
func TestNotifyValidation(t *testing.T) {
	h, key, _ := notifyFixture(t)
	cases := []struct {
		name    string
		changes map[string]string
		code    int
	}{
		{"应用不匹配", map[string]string{"app_id": "other"}, 400},
		{"收款人不匹配", map[string]string{"seller_id": "other"}, 400},
		{"事件类型错误", map[string]string{"notify_type": "other"}, 400},
		{"未知状态", map[string]string{"trade_status": "UNKNOWN"}, 400},
		{"金额格式非法", map[string]string{"total_amount": "1e2"}, 400},
		{"缺少订单号", map[string]string{"out_trade_no": ""}, 400},
		{"未付款通知仅确认接收", map[string]string{"trade_status": "WAIT_BUYER_PAY"}, 200},
		{"关闭通知不当成付款", map[string]string{"trade_status": "TRADE_CLOSED"}, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := signedValues(t, key, tc.changes)
			w := notifyRequest(h, v.Encode(), "application/x-www-form-urlencoded")
			if w.Code != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	v := signedValues(t, key, nil)
	v.Set("total_amount", "0.01")
	if w := notifyRequest(h, v.Encode(), "application/x-www-form-urlencoded"); w.Code != 400 {
		t.Fatal("篡改签名未拒绝")
	}
	v = signedValues(t, key, nil)
	v.Add("total_amount", "19.99")
	if w := notifyRequest(h, v.Encode(), "application/x-www-form-urlencoded"); w.Code != 400 {
		t.Fatal("重复参数未拒绝")
	}
	if w := notifyRequest(h, "{}", "application/json"); w.Code != 415 {
		t.Fatal(w.Code)
	}
}
func TestNotifySignedSuccessAndReplay(t *testing.T) {
	h, key, m := notifyFixture(t)
	rows := func(status string) *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "number", "book_id", "buyer_id", "seller_id", "price", "status", "payment_started_at"}).AddRow(1, "order-test", 2, 3, 4, 1999, status, time.Now())
	}
	for i := 0; i < 2; i++ {
		status, bookStatus := "pending", "reserved"
		if i == 1 {
			status, bookStatus = "paid", "sold"
		}
		m.ExpectQuery(`SELECT .*orders.*`).WillReturnRows(rows(status))
		m.ExpectQuery(`SELECT .*orders.*`).WillReturnRows(rows(status))
		m.ExpectBegin()
		m.ExpectQuery(`SELECT \* FROM books WITH`).WillReturnRows(sqlmock.NewRows([]string{"id", "seller_id", "status"}).AddRow(2, 4, bookStatus))
		m.ExpectQuery(`SELECT \* FROM orders WITH`).WillReturnRows(rows(status))
		if i == 0 {
			m.ExpectQuery(`SELECT .*payments.*`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			m.ExpectQuery(`INSERT INTO .*payments.*`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			m.ExpectExec(`UPDATE .*books.*`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`UPDATE .*orders.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		} else {
			m.ExpectQuery(`SELECT .*payments.*`).WillReturnRows(sqlmock.NewRows([]string{"order_id", "provider", "trade_no", "amount"}).AddRow(1, "alipay", "trade-1", 1999))
		}
		m.ExpectCommit()
		v := signedValues(t, key, nil)
		w := notifyRequest(h, v.Encode(), "application/x-www-form-urlencoded; charset=utf-8")
		if w.Code != 200 || w.Body.String() != "success" {
			t.Fatalf("首次或重放失败 %d %s", w.Code, w.Body.String())
		}
	}
}
func TestPaymentEndpointsIdentity(t *testing.T) {
	h, _, _ := notifyFixture(t)
	for _, method := range []gin.HandlerFunc{h.Pay, h.Cancel, h.Sync, h.Complete} {
		r := gin.New()
		r.POST("/orders/:id", method)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/orders/1", nil))
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
		r = gin.New()
		r.POST("/orders/:id", func(c *gin.Context) { c.Set("user_id", int64(3)); c.Next() }, method)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/orders/abc", nil))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}
