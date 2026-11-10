package handlers

import (
	"campus-books/models"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 保存教材接口需要使用的资源
type BookHandler struct {
	DB *gorm.DB
}

// List查询在售教材，支持按书名搜索
func (h *BookHandler) List(c *gin.Context) {
	//获取搜索关键词，去掉俩端空白
	keyword := strings.TrimSpace(c.Query("keyword"))

	//创建空切片，接收查询结果
	books := make([]models.Book, 0)

	//从结构体中获取数据库连接对象，构建条件查询
	query := h.DB.Where("status = ?", "on_sale")

	//如果有搜索关键词，则添加模糊查询条件
	if keyword != "" {
		query = query.Where("title LIKE ?", "%"+keyword+"%")
	}

	//执行查询
	result := query.Order("id DESC").Find(&books)

	if result.Error != nil {
		log.Printf("查询教材失败：%v", result.Error)

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "查询教材失败，请稍后重试",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"books": books,
	})
}
