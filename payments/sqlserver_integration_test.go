package payments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"campus-books/models"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 显式启用才运行。使用当前服务器上的独立临时数据库，不迁移或改动项目库。
// 账号无建库权限时跳过，不会退回使用项目数据库。
func TestSQLServerPaymentIntegration(t *testing.T) {
	if os.Getenv("CAMPUS_PAYMENT_SQL_TEST") != "1" {
		t.Skip("设置 CAMPUS_PAYMENT_SQL_TEST=1 才运行隔离 SQL Server 集成测试")
	}
	values, err := godotenv.Read("../.env")
	if err != nil {
		t.Fatal("无法读取本地配置")
	}
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = values["DB_DSN"]
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "sqlserver" {
		t.Fatal("集成测试需要 sqlserver URL DSN")
	}
	q := u.Query()
	q.Set("database", "master")
	q.Set("connection timeout", "5")
	u.RawQuery = q.Encode()
	admin, err := sql.Open("sqlserver", u.String())
	if err != nil {
		t.Fatal("无法初始化隔离数据库连接")
	}
	defer admin.Close()
	name := fmt.Sprintf("campus_payment_test_%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE ["+name+"]"); err != nil {
		t.Skip("不能创建隔离测试库，未触碰项目库；请在可建库的 SQL Server 环境运行")
	}
	var conn *sql.DB
	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
		// name 只由固定前缀和数字构成，删除目标只能是本测试创建的库。
		if !strings.HasPrefix(name, "campus_payment_test_") {
			panic("unexpected test database")
		}
		cleanup, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		if _, e := admin.ExecContext(cleanup, "ALTER DATABASE ["+name+"] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE ["+name+"]"); e != nil {
			t.Error("隔离测试库清理失败，请清理：", name)
		}
	}()
	q.Set("database", name)
	u.RawQuery = q.Encode()
	db, err := gorm.Open(sqlserver.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("无法连接隔离库")
	}
	conn, err = db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(10)
	if err = db.AutoMigrate(&models.User{}, &models.Book{}, &models.Order{}, &models.Payment{}); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: db}
	makeOrder := func() models.Order {
		b := models.Book{Title: "隔离测试教材", Course: "测试", Category: "其他", Condition: "九成新", Price: 1999, Contact: "测试", SellerID: 4, Status: "reserved"}
		if err := db.Create(&b).Error; err != nil {
			t.Fatal(err)
		}
		now := time.Now()
		expires := now.Add(15 * time.Minute)
		o := models.Order{Number: fmt.Sprintf("test-%d", b.ID), BookID: b.ID, BuyerID: 3, SellerID: 4, Title: b.Title, Price: b.Price, Contact: b.Contact, Status: "pending", PaymentStartedAt: &now, PaymentExpiresAt: &expires}
		if err := db.Create(&o).Error; err != nil {
			t.Fatal(err)
		}
		return o
	}
	tradeFor := func(o models.Order) *TradeResult {
		tr := paidTrade()
		tr.Number = o.Number
		tr.TradeNo = fmt.Sprintf("trade-%d", o.ID)
		return tr
	}
	t.Run("并发重复通知只入账一次", func(t *testing.T) {
		o := makeOrder()
		tr := tradeFor(o)
		var wg sync.WaitGroup
		errs := make(chan error, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- s.AcceptPaid(context.Background(), tr) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Error(err)
			}
		}
		var count int64
		db.Model(&models.Payment{}).Where("order_id = ?", o.ID).Count(&count)
		if count != 1 {
			t.Fatalf("支付记录=%d", count)
		}
		var saved models.Order
		db.First(&saved, o.ID)
		var book models.Book
		db.First(&book, o.BookID)
		if saved.Status != "paid" || book.Status != "sold" {
			t.Fatalf("订单=%s 教材=%s", saved.Status, book.Status)
		}
		if _, err := s.Complete(context.Background(), o.ID, 4); !errors.Is(err, ErrNotFound) {
			t.Fatal("卖家确认收货未被阻止", err)
		}
		done, err := s.Complete(context.Background(), o.ID, 3)
		if err != nil {
			t.Fatal(err)
		}
		again, err := s.Complete(context.Background(), o.ID, 3)
		if err != nil || !again.CompletedAt.Equal(*done.CompletedAt) {
			t.Fatal("重复收货改变完成时间", err)
		}
		if err := s.AcceptPaid(context.Background(), tr); err != nil {
			t.Fatal("完成后重复通知失败", err)
		}
	})
	t.Run("金额错误保持原状态", func(t *testing.T) {
		o := makeOrder()
		tr := tradeFor(o)
		tr.Amount = 1
		if err := s.AcceptPaid(context.Background(), tr); !errors.Is(err, ErrInvalidPayment) {
			t.Fatal(err)
		}
		var saved models.Order
		db.First(&saved, o.ID)
		if saved.Status != "pending" {
			t.Fatal(saved.Status)
		}
	})
	t.Run("冲突流水不能用于另一订单", func(t *testing.T) {
		a, b := makeOrder(), makeOrder()
		tr := tradeFor(a)
		if err := s.AcceptPaid(context.Background(), tr); err != nil {
			t.Fatal(err)
		}
		tr.Number = b.Number
		if err := s.AcceptPaid(context.Background(), tr); err == nil {
			t.Fatal("流水唯一索引未阻止重复")
		}
		var saved models.Order
		db.First(&saved, b.ID)
		var book models.Book
		db.First(&book, b.BookID)
		if saved.Status != "pending" || book.Status != "reserved" {
			t.Fatal("失败后未回滚")
		}
	})
	t.Run("并发取消与付款不会错误释放", func(t *testing.T) {
		o := makeOrder()
		s.Gateway = &fakeGateway{closeErr: ErrGateway, errorQuery: ErrGateway}
		var wg sync.WaitGroup
		wg.Add(2)
		var cancelErr, paidErr error
		go func() { defer wg.Done(); _, cancelErr = s.Cancel(context.Background(), o.ID, 3) }()
		go func() { defer wg.Done(); paidErr = s.AcceptPaid(context.Background(), tradeFor(o)) }()
		wg.Wait()
		if paidErr != nil {
			t.Fatal(paidErr)
		}
		if cancelErr == nil {
			t.Fatal("状态未知竟然取消成功")
		}
		var saved models.Order
		db.First(&saved, o.ID)
		var book models.Book
		db.First(&book, o.BookID)
		if saved.Status != "paid" || book.Status != "sold" {
			t.Fatal("付款后教材被释放")
		}
	})
	t.Run("取消旧订单不释放新占用", func(t *testing.T) {
		o := makeOrder()
		db.Model(&o).Updates(map[string]interface{}{"payment_started_at": nil, "payment_expires_at": nil})
		if _, err := s.Cancel(context.Background(), o.ID, 3); err != nil {
			t.Fatal(err)
		}
		db.Model(&models.Book{}).Where("id = ?", o.BookID).Update("status", "reserved")
		if _, err := s.Cancel(context.Background(), o.ID, 3); err != nil {
			t.Fatal(err)
		}
		var b models.Book
		db.First(&b, o.BookID)
		if b.Status != "reserved" {
			t.Fatal("旧订单释放了新的占用")
		}
	})
}
