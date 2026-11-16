package models

import "time"

// 使用常量统一订单状态，避免各个处理函数反复手写字符串。
// const 表示常量，声明后不能重新赋值。
const (
	OrderStatusPending   = "pending"   // 已下单，尚未确认付款。
	OrderStatusPaid      = "paid"      // 已确认付款，等待交接。
	OrderStatusCompleted = "completed" // 买家已经确认收到教材。
	OrderStatusCancelled = "cancelled" // 待支付订单已取消。
)

// Order 表示一笔教材交易。
// 当前设计中，一笔订单只购买一本教材。
type Order struct {
	// 数据库主键，用于接口路径中的订单编号，例如 /api/orders/12。
	ID int64 `gorm:"primaryKey" json:"id"`

	// 对外展示的订单号，例如用于订单页面和支付核对。
	// uniqueIndex 建立唯一索引，禁止两笔订单使用相同订单号。
	Number string `gorm:"type:nvarchar(64);not null;uniqueIndex" json:"number"`

	// 购买的教材编号。
	// index 为按教材查找相关订单建立索引。
	BookID int64 `gorm:"not null;index" json:"book_id"`

	// 买家编号，由下单用户的登录身份确定。
	BuyerID int64 `gorm:"not null;index" json:"buyer_id"`

	// 卖家编号，下单时从教材记录读取，不能相信客户端填写的卖家。
	SellerID int64 `gorm:"not null;index" json:"seller_id"`

	// 下单时的教材名称快照。
	// 后续教材改名，不应改变历史订单中的教材名称。
	Title string `gorm:"type:nvarchar(100);not null" json:"title"`

	// 下单时的价格快照，单位为分。
	// 支付金额应读取订单价格，不能由浏览器决定。
	Price int64 `gorm:"not null" json:"price"`

	// 下单时保存的交接联系方式。
	// 即使教材后续修改联系方式，订单仍保留原来的记录。
	Contact string `gorm:"type:nvarchar(100);not null" json:"contact"`

	// 订单状态，由后端业务逻辑控制。
	Status string `gorm:"type:nvarchar(20);not null;index" json:"status"`

	// 第一次发出付款链接的时间；为空表示从未开放过平台支付。
	// 一旦写入就不清空，取消订单时据此判断是否必须关闭支付宝交易。
	PaymentStartedAt *time.Time `json:"payment_started_at"`

	// 所有重复支付链接共用同一个绝对截止时间，避免重试延长支付窗口。
	PaymentExpiresAt *time.Time `json:"payment_expires_at"`

	// 付款确认时间，使用指针表示“可能还没有时间”。
	// 未付款时为 nil，数据库保存 NULL，JSON 返回 null。
	PaidAt *time.Time `json:"paid_at"`

	// 收货确认时间；未完成时为空。
	CompletedAt *time.Time `json:"completed_at"`

	// 取消时间；未取消时为空。
	CancelledAt *time.Time `json:"cancelled_at"`

	// GORM 按字段名约定，在创建订单时自动填写。
	CreatedAt time.Time `json:"created_at"`

	// GORM 在常规更新订单时自动维护。
	UpdatedAt time.Time `json:"updated_at"`
}
