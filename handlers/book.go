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

// CreateBookRequest 表示发布教材时允许前端提交的数据。
type CreateBookRequest struct {
	Title       string `json:"title" binding:"required,max=100"`
	Course      string `json:"course" binding:"required,max=100"`
	Condition   string `json:"condition" binding:"required,max=50"`
	Price       int64  `json:"price" binding:"gt=0"`
	Description string `json:"description" binding:"max=1000"`
	Contact     string `json:"contact" binding:"required,max=100"`
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
		pattern := "%" + keyword + "%"
		query = query.Where(
			"(title LIKE ? OR course LIKE ?)",
			pattern,
			pattern,
		)
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

// Create 发布教材，只有登录用户才能调用。
func (h *BookHandler) Create(c *gin.Context) {
	// 从认证中间件保存的上下文中读取用户编号。
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

	// 接收并校验前端提交的 JSON。
	var request CreateBookRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "请检查必填信息、字段长度和价格，价格必须为正整数，单位为分",
		})
		return
	}

	// 清理文本两端的空白。
	request.Title = strings.TrimSpace(request.Title)
	request.Course = strings.TrimSpace(request.Course)
	request.Condition = strings.TrimSpace(request.Condition)
	request.Description = strings.TrimSpace(request.Description)
	request.Contact = strings.TrimSpace(request.Contact)

	// required 无法阻止只填写空格，因此清理后再次检查。
	if request.Title == "" ||
		request.Course == "" ||
		request.Condition == "" ||
		request.Contact == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "教材名称、适用课程、成色和联系方式不能为空",
		})
		return
	}

	// 将允许提交的数据和后端确定的字段组合成教材记录。
	book := models.Book{
		Title:       request.Title,
		Course:      request.Course,
		Condition:   request.Condition,
		Price:       request.Price,
		Description: request.Description,
		Contact:     request.Contact,
		SellerID:    userID,
		Status:      "on_sale",
	}

	// 将教材保存到数据库。
	if err := h.DB.Create(&book).Error; err != nil {
		log.Printf("发布教材失败：%v", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "发布教材失败，请稍后重试",
		})
		return
	}

	// 创建成功，返回新教材的信息。
	c.JSON(http.StatusCreated, gin.H{
		"message": "发布成功",
		"data":    book,
	})
}
