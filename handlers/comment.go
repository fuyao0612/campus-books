package handlers

import (
	"campus-books/models"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 所需的数据库对象
type CommentHandler struct {
	DB *gorm.DB
}

type CommentResponse struct {
	ID        int64     `json:"id"`
	BookID    int64     `json:"book_id"`
	UserID    int64     `json:"user_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Name      string    `json:"name"`
}

// 定义前端发布留言时可以提交的数据
type CreateCommentRequest struct {
	Content string `json:"content" binding:"required,max=500"`
}

// 查询指定教材留言，按最新发布时间排序
func (h *CommentHandler) List(c *gin.Context) {
	//从URL路径中读取id参数
	bookIDText := c.Param("id")

	//转换十进制
	bookID, err := strconv.ParseInt(bookIDText, 10, 64)
	if err != nil || bookID <= 0 {
		//返回400
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "教材编号必须是正整数",
		})
		return
	}

	//在数据库中查询这本书
	var book models.Book
	result := h.DB.
		Where("id=? AND status=?", bookID, "on_sale").
		First(&book)
	//判断不存在或者已下架
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		//返回404
		c.JSON(http.StatusNotFound, gin.H{
			"message": "教材不存在或已下架",
		})
		return
	}
	//其他数据库错误
	if result.Error != nil {
		log.Printf("查询留言所属教材失败：%v", result.Error)
		//返回500
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "查询留言失败，稍后重试",
		})
		return
	}

	//留言切片
	comments := make([]CommentResponse, 0)

	//查询留言
	result = h.DB.
		Table("comments AS c").
		Select("c.id,c.book_id,c.user_id,u.name,c.content,c.created_at").
		Joins("Join users AS u ON u.id=c.user_id").
		Where("c.book_id=?", bookID).
		Order("c.created_at DESC").
		Scan(&comments)

	// 检查留言查询是否出现数据库错误。
	if result.Error != nil {
		// 把具体错误记录在后端日志，方便查明原因。
		log.Printf("查询教材留言失败：%v", result.Error)

		// 向前端返回服务器错误提示。
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "查询留言失败，请稍后重试",
		})
		// 查询失败后结束函数，不再返回成功响应。
		return
	}

	// 查询成功，返回 HTTP 200 和留言数组。
	c.JSON(http.StatusOK, gin.H{
		"message": "查询成功",   // 给前端的操作结果说明。
		"data":    comments, // 留言内容；没有留言时为空数组 []。
	})
}

// 为指定教材发布一条留言
func (h *CommentHandler) Create(c *gin.Context) {
	value, exists := c.Get("user_id")

	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "请先登录",
		})
		return
	}

	// 确认用户编号类型正确且大于零。
	userID, ok := value.(int64)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "登录信息无效，请重新登录",
		})
		return
	}

	//读取教材编号
	bookIDText := c.Param("id")
	bookID, err := strconv.ParseInt(bookIDText, 10, 64)
	if err != nil || bookID <= 0 {
		//返回400
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "教材编号必须是正整数",
		})
		return
	}

	//在数据库中查询这本书
	var book models.Book
	result := h.DB.
		Where("id=? AND status=?", bookID, "on_sale").
		First(&book)
	//判断不存在或者已下架
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		//返回404
		c.JSON(http.StatusNotFound, gin.H{
			"message": "教材不存在或已下架",
		})
		return
	}
	//其他数据库错误
	if result.Error != nil {
		log.Printf("查询留言所属教材失败：%v", result.Error)
		//返回500
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "发布留言失败，稍后重试",
		})
		return
	}

	//接收前端JSON中的content
	var request CreateCommentRequest

	//处理留言
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "留言不能为空且不能超过500个字符",
		})
		return
	}
	request.Content = strings.TrimSpace(request.Content)
	if request.Content == "" {
		// 留言清理后为空，返回 400。
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "留言内容不能为空",
		})
		// 空留言不写入数据库。
		return
	}
	comment := models.Comment{
		BookID:  bookID,
		UserID:  userID,
		Content: request.Content,
	}

	//插入数据库
	if err := h.DB.Create(&comment).Error; err != nil {
		log.Printf("保存留言失败：%v", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "发布留言失败，请稍后重试",
		})
		// 保存失败后结束处理。
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "留言发布成功",
		"data":    comment,
	})

}
