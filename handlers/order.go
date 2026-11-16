package handlers

import (
	"errors"
	"log"
	"net/http"

	"campus-books/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrderHandler 保存订单接口需要使用的数据库对象。
type OrderHandler struct {
	DB *gorm.DB
}

// 前端只提交教材编号。
// 买家、卖家、价格等信息都由后端确定。
type CreateOrderRequest struct {
	BookID int64 `json:"book_id" binding:"gt=0"`
}

// 提前定义业务错误，方便事务外判断失败原因。
// 这些错误属于包内变量，可以被本文件的方法使用。
var (
	errBookUnavailable = errors.New("教材当前不可购买")
	errBuyOwnBook      = errors.New("不能购买自己发布的教材")
)

// Create 占用教材，并创建一笔待支付订单。
func (h *OrderHandler) Create(c *gin.Context) {
	// 读取经过认证的用户编号。
	value, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "请先登录",
		})
		return
	}

	// 用户编号必须类型正确，并且为正数。
	userID, ok := value.(int64)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "登录信息无效，请重新登录",
		})
		return
	}

	// 读取请求 JSON，并检查教材编号是否为正整数。
	var request CreateOrderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "请提供有效的教材编号",
		})
		return
	}

	// 事务成功后，需要把这笔订单返回给前端。
	// 因此将变量声明在事务函数外面。
	var order models.Order

	// Transaction 开启数据库事务，并将事务对象 tx 交给函数。
	// 函数返回 nil 时提交；返回错误时回滚。
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		// 第一条写操作：只占用仍然处于在售状态的教材。
		// 条件检查和状态修改在同一条 UPDATE 中完成。
		result := tx.Model(&models.Book{}).
			Where("id = ? AND status = ?", request.BookID, "on_sale").
			Update("status", "reserved")

		// 数据库执行失败时，把错误返回给 Transaction，触发回滚。
		if result.Error != nil {
			return result.Error
		}

		// 没有更新任何记录，说明编号不存在，或教材已经不在售。
		if result.RowsAffected == 0 {
			// 再查询一次，以便区分“教材不存在”和“当前不可购买”。
			var existing models.Book
			if err := tx.First(&existing, request.BookID).Error; err != nil {
				return err
			}
			return errBookUnavailable
		}

		// 占用成功后，在同一个事务中读取教材最新信息。
		// 所有事务内操作都必须使用 tx，而不是 h.DB。
		var book models.Book
		if err := tx.First(&book, request.BookID).Error; err != nil {
			return err
		}

		// 不允许购买本人教材。
		// 返回错误会回滚刚才的占用，教材恢复为 on_sale。
		if book.SellerID == userID {
			return errBuyOwnBook
		}

		// 从数据库记录创建订单快照，不能信任客户端提供的价格。
		order = models.Order{
			Number:   uuid.NewString(),          // 生成业务订单号。
			BookID:   book.ID,                   // 所购买的教材。
			BuyerID:  userID,                    // 当前登录用户。
			SellerID: book.SellerID,             // 教材发布人。
			Title:    book.Title,                // 下单时的书名。
			Price:    book.Price,                // 下单时的价格，单位为分。
			Contact:  book.Contact,              // 下单时的联系方式。
			Status:   models.OrderStatusPending, // 初始状态为待支付。
		}

		// 插入订单，返回本次插入的错误。
		// 插入成功：返回 nil，事务提交。
		// 插入失败：返回错误，事务回滚，教材占用也随之撤销。
		return tx.Create(&order).Error
	})

	// 事务已经结束，下面统一处理 HTTP 响应。
	if err != nil {
		// 没有找到教材。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "教材不存在",
			})
			return
		}

		// 教材已下架、已售出，或者被其他订单占用。
		if errors.Is(err, errBookUnavailable) {
			c.JSON(http.StatusConflict, gin.H{
				"message": "教材当前不可购买，请刷新后重试",
			})
			return
		}

		// 用户试图购买自己发布的教材。
		if errors.Is(err, errBuyOwnBook) {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "不能购买自己发布的教材",
			})
			return
		}

		// 其他错误属于数据库或服务异常。
		log.Printf("创建订单失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "创建订单失败，请稍后重试",
		})
		return
	}

	// 只有事务提交成功，才返回创建成功。
	c.JSON(http.StatusCreated, gin.H{
		"message": "订单创建成功",
		"data":    order,
	})
}