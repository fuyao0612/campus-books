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

// ChangeBookStatusRequest 接收教材状态修改请求。
// 当前只开放下架，不允许通过这个接口直接设置已售出或重新上架。
type ChangeBookStatusRequest struct {
	// required 要求状态非空；oneof 表示只接受列出的值。
	Status string `json:"status" binding:"required,oneof=off_shelf"`
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

// 查询当前登录用户发布的教材
func (h *BookHandler) Mine(c *gin.Context) {
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

	//读取查询参数
	status := strings.TrimSpace(c.Query("status"))
	if status != "" &&
		status != "on_sale" &&
		status != "off_shelf" &&
		status != "sold" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "教材状态只能为 on_sale、off_shelf 或 sold",
		})
		return
	}

	books := make([]models.Book, 0)

	query := h.DB.Where("seller_id=?", userID)

	if status != "" {
		query = query.Where("status=?", status)
	}

	result := query.Order("id DESC").Find(&books)
	if result.Error != nil {
		log.Printf("查询本人教材失败：%v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "查询本人教材失败，请稍后重试",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查询成功",
		"data":    books,
	})
}

// 修改当前用户发布的在售教材
func (h *BookHandler) Update(c *gin.Context) {
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

	// 将路径中的教材编号转换为十进制 int64。
	bookID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || bookID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "教材编号必须是正整数",
		})
		return
	}

	// 按编号查询教材，暂时不限制状态，方便分别判断归属和状态。
	var book models.Book
	result := h.DB.First(&book, bookID)

	// 教材不存在时返回 404。
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "教材不存在",
		})
		return
	}

	// 其他数据库错误返回 500，并在终端记录具体原因。
	if result.Error != nil {
		log.Printf("查询待修改教材失败：%v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "修改教材失败，请稍后重试",
		})
		return
	}

	// 身份验证只能确认“你是谁”；这里还要判断“教材是否属于你”。
	if book.SellerID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "不能修改其他用户发布的教材",
		})
		return
	}

	// 已下架或已售出的教材不能通过这个接口修改。
	if book.Status != "on_sale" {
		c.JSON(http.StatusConflict, gin.H{
			"message": "只能修改在售教材",
		})
		return
	}

	// 编辑和发布暂时采用相同的字段及校验规则。
	var request CreateBookRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "请检查必填信息、字段长度和价格，价格必须为正整数，单位为分",
		})
		return
	}

	// 清理文本两端空白，避免保存无意义的空格。
	request.Title = strings.TrimSpace(request.Title)
	request.Course = strings.TrimSpace(request.Course)
	request.Condition = strings.TrimSpace(request.Condition)
	request.Description = strings.TrimSpace(request.Description)
	request.Contact = strings.TrimSpace(request.Contact)

	// required 无法排除纯空格文本，因此清理后再次检查必填项。
	if request.Title == "" ||
		request.Course == "" ||
		request.Condition == "" ||
		request.Contact == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "教材名称、适用课程、成色和联系方式不能为空",
		})
		return
	}

	// 显式列出允许更新的数据库列，避免意外修改发布人和状态。
	// 使用 map 更新时，description 的空字符串也会被写入数据库。
	updates := map[string]interface{}{
		"title":       request.Title,
		"course":      request.Course,
		"condition":   request.Condition,
		"price":       request.Price,
		"description": request.Description,
		"contact":     request.Contact,
	}

	// Model 指定更新教材表。
	// 更新时再次限制编号、发布人和状态，避免查询后状态变化仍被修改。
	// Updates 执行更新，并按 GORM 约定维护 UpdatedAt 时间。
	result = h.DB.Model(&models.Book{}).
		Where(
			"id = ? AND seller_id = ? AND status = ?",
			bookID,
			userID,
			"on_sale",
		).
		Updates(updates)

	// SQL 执行失败时返回服务器错误。
	if result.Error != nil {
		log.Printf("更新教材失败：%v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "修改教材失败，请稍后重试",
		})
		return
	}

	// 没有记录满足更新条件，可能是教材已被下架或删除。
	if result.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{
			"message": "教材状态已变化，请刷新后重试",
		})
		return
	}

	// 返回教材编号，前端可据此跳转到详情页并重新读取最新信息。
	c.JSON(http.StatusOK, gin.H{
		"message": "修改成功",
		"data": gin.H{
			"id": bookID,
		},
	})
}

// ChangeStatus 将当前用户发布的在售教材下架。
// 教材记录继续保留，只修改状态。
func (h *BookHandler) ChangeStatus(c *gin.Context) {
	// 从本次请求上下文中取得认证中间件保存的用户编号。
	value, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "请先登录",
		})
		return
	}

	// 类型断言：编号必须是 int64，且必须大于零。
	userID, ok := value.(int64)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "登录信息无效，请重新登录",
		})
		return
	}

	// 读取路径中的教材编号，并转换为整数。
	bookID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || bookID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "教材编号必须是正整数",
		})
		return
	}

	// 解析请求体，同时执行 required 和 oneof 校验。
	var request ChangeBookStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "当前只允许将状态设置为 off_shelf",
		})
		return
	}

	// 查询教材，准备检查它是否存在、属于谁以及当前状态。
	var book models.Book
	result := h.DB.First(&book, bookID)

	// 没有找到教材时返回 404。
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "教材不存在",
		})
		return
	}

	// 数据库查询失败时记录详细错误，向客户端返回 500。
	if result.Error != nil {
		log.Printf("查询待下架教材失败：%v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "下架教材失败，请稍后重试",
		})
		return
	}

	// 先检查归属，禁止用户操作他人的教材。
	if book.SellerID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "不能下架其他用户发布的教材",
		})
		return
	}

	// 已经下架时直接返回成功，不必重复更新数据库。
	if book.Status == "off_shelf" {
		c.JSON(http.StatusOK, gin.H{
			"message": "教材已下架",
			"data": gin.H{
				"id":     bookID,
				"status": "off_shelf",
			},
		})
		return
	}

	// 已售出或其他非在售状态不允许执行下架。
	if book.Status != "on_sale" {
		c.JSON(http.StatusConflict, gin.H{
			"message": "只能下架在售教材",
		})
		return
	}

	// 执行更新时再次限制编号、发布人和当前状态。
	// Update 只修改指定字段，并由 GORM 自动维护 UpdatedAt。
	result = h.DB.Model(&models.Book{}).
		Where(
			"id = ? AND seller_id = ? AND status = ?",
			bookID,
			userID,
			"on_sale",
		).
		Update("status", request.Status)

	// 更新执行失败，返回 500。
	if result.Error != nil {
		log.Printf("下架教材失败：%v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "下架教材失败，请稍后重试",
		})
		return
	}

	// 查询和更新之间教材状态可能变化；零行更新说明条件已不满足。
	if result.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{
			"message": "教材状态已变化，请刷新后重试",
		})
		return
	}

	// 更新成功，返回教材编号和新的状态。
	c.JSON(http.StatusOK, gin.H{
		"message": "下架成功",
		"data": gin.H{
			"id":     bookID,
			"status": request.Status,
		},
	})
}
