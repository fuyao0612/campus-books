package handlers

import (
	"campus-books/models"
	"errors"
	"log"
	"net/http"
	"strings"

	"strconv"

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
		"message": "查询成功",
		"data":    books,
	})
}

// Detail根据教材编号查询教材详情
func (h *BookHandler) Detail(c *gin.Context) {
	//获取路径参数，如/api/books/1中的1
	idText := c.Param("id")

	//将字符串编号转换为int64
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "教材编号必须是正整数",
		})
		return
	}

	//创建结构体变量，接收查到的教材
	var book models.Book

	//只查询指定编号在售的教材
	result := h.DB.
		Where("id=? AND status=?", id, "on_sale").
		First(&book)

	//区分没有找到教材和其他的数据库错误
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "教材不存在或已下架",
		})
		return
	}

	if result.Error != nil {
		log.Printf("查询教材详情失败:%v", result.Error)

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "查询教材详情失败，请稍后尝试",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查询成功",
		"data":    book,
	})

}
