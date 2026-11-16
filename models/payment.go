package models

import "time"

// Payment 表示一笔经过后端核验的成功支付。
// 发起支付请求时不创建成功记录；确认付款后才保存。
type Payment struct {
	// 数据库主键，标识这条支付记录。
	ID int64 `gorm:"primaryKey" json:"id"`

	// 关联本系统的订单编号。
	// 当前一笔订单只允许有一条成功支付记录。
	// uniqueIndex 在数据库层阻止同一订单重复插入成功记录。
	OrderID int64 `gorm:"not null;uniqueIndex" json:"order_id"`

	// 支付渠道，接入支付宝时保存 alipay。
	// 与 TradeNo 共同组成唯一索引。
	Provider string `gorm:"type:nvarchar(20);not null;uniqueIndex:idx_payment_provider_trade" json:"provider"`

	// 支付平台生成的交易流水号。
	// 它不是本系统的 Order.Number，两者用于相互核对。
	TradeNo string `gorm:"type:nvarchar(64);not null;uniqueIndex:idx_payment_provider_trade" json:"trade_no"`

	// 实际核验的支付金额，单位为分。
	// 必须与订单中的 Price 一致，不能相信浏览器提交的金额。
	Amount int64 `gorm:"not null" json:"amount"`

	// 支付成功时间，后续从核验通过的平台结果中取得。
	PaidAt time.Time `json:"paid_at"`

	// 本系统保存这条记录的时间，由 GORM 自动填写。
	// 它可能晚于平台付款时间，例如通知存在延迟。
	CreatedAt time.Time `json:"created_at"`
}
