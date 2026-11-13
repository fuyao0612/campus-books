package models

import (
	"time"
)

type Comment struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	BookID    int64     `gorm:"not null;index" json:"book_id"`
	UserID    int64     `gorm:"not null;index" json:"user_id"`
	Content   string    `gorm:"type:nvarchar(500);not null" json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
