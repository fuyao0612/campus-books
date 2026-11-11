package models

import "time"

type User struct {
	//用户编号，数据库中的主键
	ID int64 `gorm:"primaryKey" json:"id"`

	//学生姓名，登录时作为用户名
	Name string `gorm:"type:nvarchar(50);not null" json:"name"`

	//密码哈希，学号作为密码
	PasswordHash string `gorm:"type:nvarchar(255);not null" json:"-"`

	//用户创建时间
	CreatedAt time.Time `json:"created_at"`

	//用户最后更新时间
	UpdatedAt time.Time `json:"updated_at"`
}
