package models

import "time"

// Book 表示一条二手教材信息。
type Book struct {
	//教材编号
	ID int64 `gorm:"primaryKey" json:"id"`

	//教材名称，类型为支持中文的字符串类型
	Title string `gorm:"type:nvarchar(100);not null" json:"title"`

	//教材适用课程，类型为支持中文的字符串类型
	Course string `gorm:"type:nvarchar(100);not null" json:"course"`

	//教材成色
	Condition string `gorm:"type:nvarchar(50);not null" json:"condition"`

	//教材价格，类型为整数，单位为分
	Price int64 `gorm:"not null" json:"price"`

	//教材描述补充说明，类型为支持中文的字符串类型
	Description string `gorm:"type:nvarchar(1000)" json:"description"`

	// 发布人联系方式
	Contact string `gorm:"type:nvarchar(100);not null" json:"contact"`

	// 卖家ID
	SellerID int64 `gorm:"not null" json:"seller_id"`

	// 教材状态
	Status string `gorm:"type:nvarchar(20);not null" json:"status"`

	// 创建时间
	CreatedAt time.Time `json:"created_at"`

	// 教材最后信息的更新时间
	UpdatedAt time.Time `json:"updated_at"`
}
