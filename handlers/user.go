package handlers

import (
	"campus-books/models"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

//返回当前用户信息
func (h *AuthHandler) Me(c *gin.Context) {
	//从认证中间件保存的上下文中取得用户编号
	value, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "请先登录",
		})
		return
	}

	// 确认取出的值是 int64 类型。
	userID, ok := value.(int64)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "登录信息无效，请重新登录",
		})
		return
	}

	// 根据令牌中的用户编号查询数据库。
	var user models.User
	result := h.DB.First(&user, userID)

	// 令牌尚未过期，但对应账号可能已经被删除。
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "用户不存在，请重新登录",
		})
		return
	}

	if result.Error != nil {
		log.Printf("查询当前用户失败：%v", result.Error)

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "查询用户失败，请稍后重试",
		})
		return
	}

	// 明确选择可以返回给前端的字段。
	c.JSON(http.StatusOK, gin.H{
		"message": "查询成功",
		"data": gin.H{
			"id":         user.ID,
			"name":       user.Name,
			"created_at": user.CreatedAt,
		},
	})
}
