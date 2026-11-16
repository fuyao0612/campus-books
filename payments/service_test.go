package payments

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"testing"
	"time"

	"campus-books/config"
	"campus-books/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/smartwalle/alipay/v3"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlmock 不访问真实数据，用于明确验证SQL失败时是否提交或回滚。
func mockService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
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
	return &Service{DB: db, Config: &config.AlipayConfig{Sandbox: true, NotifyURL: "https://public.example/api/payments/alipay/notify", ReturnBaseURL: "http://127.0.0.1:5174/payment.html", SellerID: "seller"}}, m
}
func orderRows(status string, started bool) *sqlmock.Rows {
	var start, expires interface{}
	if started {
		start = time.Date(2026, 11, 16, 12, 0, 0, 0, chinaTime)
		expires = time.Date(2026, 11, 16, 12, 15, 0, 0, chinaTime)
	}
	return sqlmock.NewRows([]string{"id", "number", "book_id", "buyer_id", "seller_id", "title", "price", "contact", "status", "payment_started_at", "payment_expires_at"}).AddRow(1, "order-test", 2, 3, 4, "测试书", 1999, "联系", status, start, expires)
}
func expectLocked(m sqlmock.Sqlmock, status, bookStatus string, started bool) {
	m.ExpectQuery(`SELECT .*orders.*`).WillReturnRows(orderRows(status, started))
	m.ExpectBegin()
	m.ExpectQuery(regexp.QuoteMeta("SELECT * FROM books WITH (UPDLOCK, ROWLOCK) WHERE id = @p1")).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"id", "seller_id", "status"}).AddRow(2, 4, bookStatus))
	m.ExpectQuery(regexp.QuoteMeta("SELECT * FROM orders WITH (UPDLOCK, ROWLOCK) WHERE id = @p1")).WithArgs(int64(1)).WillReturnRows(orderRows(status, started))
}
func expectEmptyPayment(m sqlmock.Sqlmock) {
	m.ExpectQuery(`SELECT .*payments.*`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
}
func paidTrade() *TradeResult {
	return &TradeResult{Number: "order-test", TradeNo: "trade-1", Amount: 1999, Status: alipay.TradeStatusSuccess, PaidAt: "2026-11-16 12:05:00"}
}

type fakeGateway struct {
	closeErr, errorQuery   error
	trade                  *TradeResult
	closeCalls, queryCalls int
	page                   alipay.TradePagePay
}

func (f *fakeGateway) PagePay(p alipay.TradePagePay) (*url.URL, error) {
	f.page = p
	return url.Parse("https://sandbox.example/pay")
}
func (f *fakeGateway) Close(context.Context, string) error { f.closeCalls++; return f.closeErr }
func (f *fakeGateway) Query(context.Context, string) (*TradeResult, error) {
	f.queryCalls++
	return f.trade, f.errorQuery
}

func TestPaymentTransactionBranches(t *testing.T) {
	t.Run("错误金额不写数据库", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "pending", "reserved", true)
		m.ExpectRollback()
		tr := paidTrade()
		tr.Amount = 1
		_, err := s.withOrder(context.Background(), 1, 3, true, func(tx *gorm.DB, o *models.Order, b *models.Book) error { return s.settle(tx, o, b, tr) })
		if !errors.Is(err, ErrInvalidPayment) {
			t.Fatal(err)
		}
	})
	t.Run("首次成功原子入账", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "pending", "reserved", true)
		expectEmptyPayment(m)
		m.ExpectQuery(`INSERT INTO .*payments.*`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10))
		m.ExpectExec(`UPDATE .*books.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectExec(`UPDATE .*orders.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectCommit()
		o, err := s.withOrder(context.Background(), 1, 3, true, func(tx *gorm.DB, o *models.Order, b *models.Book) error { return s.settle(tx, o, b, paidTrade()) })
		if err != nil || o.Status != "paid" {
			t.Fatalf("%+v %v", o, err)
		}
	})
	t.Run("支付记录写入失败回滚", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "pending", "reserved", true)
		expectEmptyPayment(m)
		m.ExpectQuery(`INSERT INTO .*payments.*`).WillReturnError(errors.New("写入失败"))
		m.ExpectRollback()
		_, err := s.withOrder(context.Background(), 1, 3, true, func(tx *gorm.DB, o *models.Order, b *models.Book) error { return s.settle(tx, o, b, paidTrade()) })
		if err == nil {
			t.Fatal("应当回滚")
		}
	})
	t.Run("订单更新失败全部回滚", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "pending", "reserved", true)
		expectEmptyPayment(m)
		m.ExpectQuery(`INSERT INTO .*payments.*`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10))
		m.ExpectExec(`UPDATE .*books.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectExec(`UPDATE .*orders.*`).WillReturnError(errors.New("更新失败"))
		m.ExpectRollback()
		_, err := s.withOrder(context.Background(), 1, 3, true, func(tx *gorm.DB, o *models.Order, b *models.Book) error { return s.settle(tx, o, b, paidTrade()) })
		if err == nil {
			t.Fatal("应当回滚")
		}
	})
	for _, status := range []string{"paid", "completed"} {
		t.Run("重复通知保持"+status, func(t *testing.T) {
			s, m := mockService(t)
			expectLocked(m, status, "sold", true)
			m.ExpectQuery(`SELECT .*payments.*`).WillReturnRows(sqlmock.NewRows([]string{"order_id", "provider", "trade_no", "amount"}).AddRow(1, "alipay", "trade-1", 1999))
			m.ExpectCommit()
			_, err := s.withOrder(context.Background(), 1, 3, true, func(tx *gorm.DB, o *models.Order, b *models.Book) error { return s.settle(tx, o, b, paidTrade()) })
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("重复通知流水不符拒绝", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "paid", "sold", true)
		m.ExpectQuery(`SELECT .*payments.*`).WillReturnRows(sqlmock.NewRows([]string{"order_id", "provider", "trade_no", "amount"}).AddRow(1, "alipay", "other", 1999))
		m.ExpectRollback()
		_, err := s.withOrder(context.Background(), 1, 3, true, func(tx *gorm.DB, o *models.Order, b *models.Book) error { return s.settle(tx, o, b, paidTrade()) })
		if !errors.Is(err, ErrInvalidPayment) {
			t.Fatal(err)
		}
	})
	t.Run("已取消订单不能入账", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "cancelled", "reserved", true)
		expectEmptyPayment(m)
		m.ExpectRollback()
		_, err := s.withOrder(context.Background(), 1, 3, true, func(tx *gorm.DB, o *models.Order, b *models.Book) error { return s.settle(tx, o, b, paidTrade()) })
		if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	})
}
func TestCancelAndSyncBranches(t *testing.T) {
	t.Run("未发链直接取消", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "pending", "reserved", false)
		m.ExpectExec(`UPDATE .*books.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectExec(`UPDATE .*orders.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectCommit()
		o, err := s.Cancel(context.Background(), 1, 3)
		if err != nil || o.Status != "cancelled" {
			t.Fatalf("%+v %v", o, err)
		}
	})
	t.Run("旧取消不释放新订单教材", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "cancelled", "reserved", true)
		m.ExpectCommit()
		if _, err := s.Cancel(context.Background(), 1, 3); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("已发链先关闭平台", func(t *testing.T) {
		s, m := mockService(t)
		f := &fakeGateway{}
		s.Gateway = f
		expectLocked(m, "pending", "reserved", true)
		m.ExpectExec(`UPDATE .*books.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectExec(`UPDATE .*orders.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectCommit()
		if _, err := s.Cancel(context.Background(), 1, 3); err != nil {
			t.Fatal(err)
		}
		if f.closeCalls != 1 {
			t.Fatal("没有关闭平台")
		}
	})
	t.Run("平台超时保持占用", func(t *testing.T) {
		s, m := mockService(t)
		s.Gateway = &fakeGateway{closeErr: ErrGateway, errorQuery: ErrGateway}
		expectLocked(m, "pending", "reserved", true)
		m.ExpectRollback()
		if _, err := s.Cancel(context.Background(), 1, 3); !errors.Is(err, ErrGateway) {
			t.Fatal(err)
		}
	})
	t.Run("有效旧链接且无平台交易不释放", func(t *testing.T) {
		s, m := mockService(t)
		s.Now = func() time.Time { return time.Date(2026, 11, 16, 12, 5, 0, 0, chinaTime) }
		s.Gateway = &fakeGateway{closeErr: ErrGateway, errorQuery: ErrTradeMissing}
		expectLocked(m, "pending", "reserved", true)
		m.ExpectRollback()
		if _, err := s.Cancel(context.Background(), 1, 3); !errors.Is(err, ErrAwaitExpiry) {
			t.Fatal(err)
		}
	})
	t.Run("链接过期且平台确认无交易才释放", func(t *testing.T) {
		s, m := mockService(t)
		s.Now = func() time.Time { return time.Date(2026, 11, 16, 12, 17, 0, 0, chinaTime) }
		s.Gateway = &fakeGateway{closeErr: ErrGateway, errorQuery: ErrTradeMissing}
		expectLocked(m, "pending", "reserved", true)
		m.ExpectExec(`UPDATE .*books.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectExec(`UPDATE .*orders.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectCommit()
		if _, err := s.Cancel(context.Background(), 1, 3); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("取消时已付款同步入账但不取消", func(t *testing.T) {
		s, m := mockService(t)
		s.Gateway = &fakeGateway{closeErr: ErrGateway, trade: paidTrade()}
		expectLocked(m, "pending", "reserved", true)
		expectEmptyPayment(m)
		m.ExpectQuery(`INSERT INTO .*payments.*`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10))
		m.ExpectExec(`UPDATE .*books.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectExec(`UPDATE .*orders.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectCommit()
		o, err := s.Cancel(context.Background(), 1, 3)
		if !errors.Is(err, ErrAlreadyPaid) || o.Status != "paid" {
			t.Fatalf("%+v %v", o, err)
		}
	})
	t.Run("卖家不能取消", func(t *testing.T) {
		s, m := mockService(t)
		m.ExpectQuery(`SELECT .*orders.*`).WillReturnRows(orderRows("pending", true))
		if _, err := s.Cancel(context.Background(), 1, 4); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	})
	t.Run("重复收货保留完成状态", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "completed", "sold", true)
		m.ExpectCommit()
		if _, err := s.Complete(context.Background(), 1, 3); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("未付款不能确认收货", func(t *testing.T) {
		s, m := mockService(t)
		expectLocked(m, "pending", "reserved", true)
		m.ExpectRollback()
		if _, err := s.Complete(context.Background(), 1, 3); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	})
	t.Run("主动查单缺失不误判失败", func(t *testing.T) {
		s, m := mockService(t)
		s.Gateway = &fakeGateway{errorQuery: ErrTradeMissing}
		expectLocked(m, "pending", "reserved", true)
		m.ExpectCommit()
		o, err := s.Sync(context.Background(), 1, 3)
		if err != nil || o.Status != "pending" {
			t.Fatalf("%+v %v", o, err)
		}
	})
}

func TestPayWindowAndPermissions(t *testing.T) {
	t.Run("首次发链先持久化固定截止时间", func(t *testing.T) {
		s, m := mockService(t)
		g := &fakeGateway{}
		s.Gateway = g
		s.Now = func() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, chinaTime) }
		expectLocked(m, "pending", "reserved", false)
		m.ExpectExec(`UPDATE .*orders.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectCommit()
		address, err := s.Pay(context.Background(), 1, 3)
		if err != nil || address == "" {
			t.Fatal(err)
		}
		if g.page.TotalAmount != "19.99" || g.page.TimeExpire != "2026-11-16 12:15:00" {
			t.Fatalf("%+v", g.page.Trade)
		}
		back, _ := url.Parse(g.page.ReturnURL)
		if back.Query().Get("id") != "1" || back.Query().Get("return") != "1" {
			t.Fatal(back)
		}
	})
	t.Run("重试不延长付款窗口", func(t *testing.T) {
		s, m := mockService(t)
		g := &fakeGateway{}
		s.Gateway = g
		s.Now = func() time.Time { return time.Date(2026, 11, 16, 12, 5, 0, 0, chinaTime) }
		expectLocked(m, "pending", "reserved", true)
		m.ExpectCommit()
		if _, err := s.Pay(context.Background(), 1, 3); err != nil {
			t.Fatal(err)
		}
		if g.page.TimeExpire != "2026-11-16 12:15:00" {
			t.Fatal("窗口被延长")
		}
	})
	t.Run("已过期不再发链", func(t *testing.T) {
		s, m := mockService(t)
		s.Gateway = &fakeGateway{}
		s.Now = func() time.Time { return time.Date(2026, 11, 16, 12, 16, 0, 0, chinaTime) }
		expectLocked(m, "pending", "reserved", true)
		m.ExpectRollback()
		if _, err := s.Pay(context.Background(), 1, 3); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	})
	t.Run("卖家不能替买家支付", func(t *testing.T) {
		s, m := mockService(t)
		s.Gateway = &fakeGateway{}
		m.ExpectQuery(`SELECT .*orders.*`).WillReturnRows(orderRows("pending", true))
		if _, err := s.Pay(context.Background(), 1, 4); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	})
	t.Run("主动查到关闭才释放", func(t *testing.T) {
		s, m := mockService(t)
		tr := paidTrade()
		tr.Status = alipay.TradeStatusClosed
		s.Gateway = &fakeGateway{trade: tr}
		expectLocked(m, "pending", "reserved", true)
		m.ExpectExec(`UPDATE .*books.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectExec(`UPDATE .*orders.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectCommit()
		o, err := s.Sync(context.Background(), 1, 3)
		if err != nil || o.Status != "cancelled" {
			t.Fatalf("%+v %v", o, err)
		}
	})
}
