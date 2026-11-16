package payments

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"campus-books/config"
	"campus-books/models"
	"github.com/smartwalle/alipay/v3"
	"gorm.io/gorm"
)

// Service 管理支付、取消和入账；所有状态改变都经过同一套事务规则。
type Service struct {
	DB      *gorm.DB
	Gateway Gateway
	Config  *config.AlipayConfig
	Now     func() time.Time // 测试可固定时间；真实运行使用 time.Now。
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var chinaTime = time.FixedZone("Asia/Shanghai", 8*60*60)

// lockRecord 使用 SQL Server 更新锁；不要换成该驱动不支持的 FOR UPDATE。
// 事务中统一“先锁教材，再锁订单”，与下单时先占用教材的顺序一致。
func lockRecord(tx *gorm.DB, table string, id int64, dest interface{}) error {
	if table != "books" && table != "orders" {
		return fmt.Errorf("invalid lock table")
	}
	r := tx.Raw("SELECT * FROM "+table+" WITH (UPDLOCK, ROWLOCK) WHERE id = ?", id).Scan(dest)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// withOrder 先读取不可由用户修改的教材关联，再进入事务取得更新锁并重新检查。
func (s *Service) withOrder(ctx context.Context, id, userID int64, buyerOnly bool, work func(*gorm.DB, *models.Order, *models.Book) error) (*models.Order, error) {
	var initial models.Order
	if err := s.DB.WithContext(ctx).First(&initial, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	// userID=0 仅供已完成平台验签的通知流程内部使用。
	if userID != 0 && !canAccess(initial, userID, buyerOnly) {
		return nil, ErrNotFound
	}
	var order models.Order
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var book models.Book
		if err := lockRecord(tx, "books", initial.BookID, &book); err != nil {
			return err
		}
		if err := lockRecord(tx, "orders", id, &order); err != nil {
			return err
		}
		if order.BookID != book.ID || order.SellerID != book.SellerID {
			return ErrConflict
		}
		if userID != 0 && !canAccess(order, userID, buyerOnly) {
			return ErrNotFound
		}
		return work(tx, &order, &book)
	})
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// Pay 返回签名后的沙箱付款地址；记录发链时间成功提交后，才把地址交给浏览器。
func (s *Service) Pay(ctx context.Context, id, userID int64) (string, error) {
	if s.Config == nil || !s.Config.Sandbox || s.Gateway == nil {
		return "", ErrGateway
	}
	if err := ValidateNotifyURL(s.Config.NotifyURL); err != nil {
		return "", err
	}
	var paymentURL string
	_, err := s.withOrder(ctx, id, userID, true, func(tx *gorm.DB, order *models.Order, book *models.Book) error {
		if order.Status != models.OrderStatusPending || book.Status != "reserved" {
			return ErrConflict
		}
		amount, err := CentsToYuan(order.Price)
		if err != nil {
			return err
		}
		now := s.now()
		if order.PaymentStartedAt == nil {
			// 第一次发链时建立固定15分钟窗口，重试不会延长窗口。
			expires := now.Add(15 * time.Minute).Truncate(time.Second)
			if err := tx.Model(order).Updates(map[string]interface{}{"payment_started_at": now, "payment_expires_at": expires}).Error; err != nil {
				return err
			}
			order.PaymentStartedAt, order.PaymentExpiresAt = &now, &expires
		}
		if order.PaymentExpiresAt == nil || !now.Before(*order.PaymentExpiresAt) {
			return fmt.Errorf("%w：付款链接已到期，请取消后重新下单", ErrConflict)
		}
		back, err := url.Parse(s.Config.ReturnBaseURL)
		if err != nil {
			return err
		}
		q := back.Query()
		q.Set("id", strconv.FormatInt(order.ID, 10))
		q.Set("return", "1")
		back.RawQuery = q.Encode()
		// TimeExpire 是绝对截止时间；旧链接也不能无限期发起付款。
		p := alipay.TradePagePay{Trade: alipay.Trade{
			Subject: order.Title, OutTradeNo: order.Number, TotalAmount: amount,
			ProductCode: "FAST_INSTANT_TRADE_PAY", SellerId: s.Config.SellerID,
			NotifyURL: s.Config.NotifyURL, ReturnURL: back.String(),
			TimeExpire: order.PaymentExpiresAt.In(chinaTime).Format("2006-01-02 15:04:05"),
		}}
		u, err := s.Gateway.PagePay(p)
		if err != nil {
			return err
		}
		if u == nil || u.Scheme != "https" {
			return ErrGateway
		}
		paymentURL = u.String()
		return nil
	})
	return paymentURL, err
}

// settle 在已经持有教材和订单锁的事务内执行，保证重复通知只产生一条记录。
func (s *Service) settle(tx *gorm.DB, order *models.Order, book *models.Book, trade *TradeResult) error {
	if trade == nil || trade.Number != order.Number || trade.Amount != order.Price || trade.TradeNo == "" {
		return ErrInvalidPayment
	}
	if trade.Status != alipay.TradeStatusSuccess && trade.Status != alipay.TradeStatusFinished {
		return ErrInvalidPayment
	}
	var saved models.Payment
	err := tx.Where("order_id = ?", order.ID).Take(&saved).Error
	if err == nil {
		// 不能把“已存在任意支付记录”都当作成功，必须逐项比对。
		if saved.Provider != "alipay" || saved.TradeNo != trade.TradeNo || saved.Amount != trade.Amount ||
			(order.Status != models.OrderStatusPaid && order.Status != models.OrderStatusCompleted) || book.Status != "sold" {
			return ErrInvalidPayment
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if order.Status != models.OrderStatusPending || book.Status != "reserved" || order.PaymentStartedAt == nil {
		return ErrConflict
	}
	paidAt, err := time.ParseInLocation("2006-01-02 15:04:05", trade.PaidAt, chinaTime)
	if err != nil {
		return fmt.Errorf("%w：付款时间格式错误", ErrInvalidPayment)
	}
	payment := models.Payment{OrderID: order.ID, Provider: "alipay", TradeNo: trade.TradeNo, Amount: trade.Amount, PaidAt: paidAt}
	// 两个唯一索引防止订单或平台流水被重复入账；任何失败都回滚全部操作。
	if err := tx.Create(&payment).Error; err != nil {
		return err
	}
	if err := tx.Model(book).Update("status", "sold").Error; err != nil {
		return err
	}
	if err := tx.Model(order).Updates(map[string]interface{}{"status": models.OrderStatusPaid, "paid_at": paidAt}).Error; err != nil {
		return err
	}
	book.Status, order.Status, order.PaidAt = "sold", models.OrderStatusPaid, &paidAt
	return nil
}

// AcceptPaid 仅由已验签的通知调用，订单号使用业务 Number 而非主键。
func (s *Service) AcceptPaid(ctx context.Context, trade *TradeResult) error {
	var order models.Order
	if trade == nil {
		return ErrInvalidPayment
	}
	if err := s.DB.WithContext(ctx).Where("number = ?", trade.Number).Take(&order).Error; err != nil {
		return err
	}
	_, err := s.withOrder(ctx, order.ID, 0, false, func(tx *gorm.DB, order *models.Order, book *models.Book) error {
		return s.settle(tx, order, book, trade)
	})
	return err
}

func (s *Service) cancelLocal(tx *gorm.DB, order *models.Order, book *models.Book) error {
	if order.Status != models.OrderStatusPending || book.Status != "reserved" {
		return ErrConflict
	}
	now := s.now()
	if err := tx.Model(book).Update("status", "on_sale").Error; err != nil {
		return err
	}
	if err := tx.Model(order).Updates(map[string]interface{}{"status": models.OrderStatusCancelled, "cancelled_at": now}).Error; err != nil {
		return err
	}
	book.Status, order.Status, order.CancelledAt = "on_sale", models.OrderStatusCancelled, &now
	return nil
}

// Cancel：从未发链可直接取消；发过链接则必须协调平台关闭。
func (s *Service) Cancel(ctx context.Context, id, userID int64) (*models.Order, error) {
	alreadyPaid := false
	order, err := s.withOrder(ctx, id, userID, true, func(tx *gorm.DB, order *models.Order, book *models.Book) error {
		if order.Status == models.OrderStatusCancelled {
			return nil
		} // 不释放后来新订单占用的教材。
		if order.Status != models.OrderStatusPending {
			return ErrConflict
		}
		if order.PaymentStartedAt == nil {
			return s.cancelLocal(tx, order, book)
		}
		if s.Gateway == nil {
			return ErrGateway
		}
		// 关闭成功后旧链接不能再付款；如果请求超时，继续查询真实状态。
		closeErr := s.Gateway.Close(ctx, order.Number)
		if closeErr == nil {
			return s.cancelLocal(tx, order, book)
		}
		trade, err := s.Gateway.Query(ctx, order.Number)
		if errors.Is(err, ErrTradeMissing) {
			// 发出的URL可能尚未打开，平台无交易不等于今后不会创建。
			// 仅在绝对有效期过去且额外等待一分钟后，才释放无平台交易的教材。
			if order.PaymentExpiresAt != nil && s.now().After(order.PaymentExpiresAt.Add(time.Minute)) {
				return s.cancelLocal(tx, order, book)
			}
			return ErrAwaitExpiry
		}
		if err != nil {
			return err
		}
		if trade.Number != order.Number || trade.Amount != order.Price {
			return ErrInvalidPayment
		}
		switch trade.Status {
		case alipay.TradeStatusSuccess, alipay.TradeStatusFinished:
			if err := s.settle(tx, order, book, trade); err != nil {
				return err
			}
			alreadyPaid = true
			return nil // 付款入账必须提交，不能通过返回错误把它回滚。
		case alipay.TradeStatusClosed:
			return s.cancelLocal(tx, order, book)
		default:
			return ErrGateway // 未确认平台关闭，不释放教材。
		}
	})
	if err == nil && alreadyPaid {
		return order, ErrAlreadyPaid
	}
	return order, err
}

// Sync 从平台主动查询；供付款返回、通知延迟或关闭请求超时后的恢复使用。
func (s *Service) Sync(ctx context.Context, id, userID int64) (*models.Order, error) {
	return s.withOrder(ctx, id, userID, false, func(tx *gorm.DB, order *models.Order, book *models.Book) error {
		if order.Status != models.OrderStatusPending || order.PaymentStartedAt == nil {
			return nil
		}
		if s.Gateway == nil {
			return ErrGateway
		}
		trade, err := s.Gateway.Query(ctx, order.Number)
		if errors.Is(err, ErrTradeMissing) {
			return nil
		} // 尚未进入付款页，保持待支付。
		if err != nil {
			return err
		}
		if trade.Number != order.Number || trade.Amount != order.Price {
			return ErrInvalidPayment
		}
		switch trade.Status {
		case alipay.TradeStatusSuccess, alipay.TradeStatusFinished:
			return s.settle(tx, order, book, trade)
		case alipay.TradeStatusClosed:
			return s.cancelLocal(tx, order, book)
		case alipay.TradeStatusWaitBuyerPay:
			return nil
		default:
			return ErrInvalidPayment
		}
	})
}

// Complete 只允许买家确认已支付订单，重复确认保持原完成时间。
func (s *Service) Complete(ctx context.Context, id, userID int64) (*models.Order, error) {
	return s.withOrder(ctx, id, userID, true, func(tx *gorm.DB, order *models.Order, book *models.Book) error {
		if order.Status == models.OrderStatusCompleted {
			return nil
		}
		if order.Status != models.OrderStatusPaid || book.Status != "sold" {
			return ErrConflict
		}
		var payment models.Payment
		if err := tx.Where("order_id = ?", order.ID).Take(&payment).Error; err != nil {
			return err
		}
		if payment.Amount != order.Price || payment.Provider != "alipay" {
			return ErrInvalidPayment
		}
		now := s.now()
		if err := tx.Model(order).Updates(map[string]interface{}{"status": models.OrderStatusCompleted, "completed_at": now}).Error; err != nil {
			return err
		}
		order.Status, order.CompletedAt = models.OrderStatusCompleted, &now
		return nil
	})
}
