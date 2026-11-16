package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// OrderResponse 返回订单快照，以及买卖双方的姓名。
// 姓名查询自用户表，不需要额外保存到订单表。
type OrderResponse struct {
	models.Order `gorm:"embedded"` // 复用订单的全部字段。

	BuyerName  string `json:"buyer_name"`  // 买家当前姓名。
	SellerName string `json:"seller_name"` // 卖家当前姓名。
}

// queryWithNames 构建带买卖双方姓名的订单查询。
// 只构建查询，不会立即访问数据库。
func (h *OrderHandler) queryWithNames() *gorm.DB {
	return h.DB.
		Table("orders AS o").
		Select("o.*, COALESCE(buyer.name, '') AS buyer_name, COALESCE(seller.name, '') AS seller_name").
		Joins("LEFT JOIN users AS buyer ON buyer.id = o.buyer_id").
		Joins("LEFT JOIN users AS seller ON seller.id = o.seller_id")
}

// 提前定义业务错误，方便事务外判断失败原因。
// 这些错误属于包内变量，可以被本文件的方法使用。
var (
	errBookUnavailable = errors.New("教材当前不可购买")
	errBuyOwnBook      = errors.New("不能购买自己发布的教材")
	// 订单不再是待支付状态，或者处理过程中状态发生变化。
	errOrderConflict = errors.New("订单状态不允许取消")

	// 教材状态与订单预期不一致，不能直接释放教材。
	errOrderBookConflict = errors.New("订单关联的教材状态异常")
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

// List 查询当前用户买入或卖出的订单，支持状态筛选。
func (h *OrderHandler) List(c *gin.Context) {
	// 从认证中间件保存的上下文取得用户编号。
	value, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "请先登录"})
		return
	}

	// 确认用户编号的具体类型和值有效。
	userID, ok := value.(int64)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "登录信息无效，请重新登录"})
		return
	}

	// role 用来区分买入与卖出；不传时默认查询买入订单。
	role := strings.TrimSpace(c.Query("role"))
	if role == "" {
		role = "buy"
	}

	// 只接受约定的两种角色。
	if role != "buy" && role != "sell" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "订单角色只能为 buy 或 sell"})
		return
	}

	// status 可以省略；填写时只允许已有订单状态。
	status := strings.TrimSpace(c.Query("status"))
	if status != "" &&
		status != models.OrderStatusPending &&
		status != models.OrderStatusPaid &&
		status != models.OrderStatusCompleted &&
		status != models.OrderStatusCancelled {
		c.JSON(http.StatusBadRequest, gin.H{"message": "订单状态不正确"})
		return
	}

	// 先构建关联姓名的查询，再根据角色限定当前用户。
	query := h.queryWithNames()
	if role == "buy" {
		// 买入：订单中的买家必须是当前用户。
		query = query.Where("o.buyer_id = ?", userID)
	} else {
		// 卖出：订单中的卖家必须是当前用户。
		query = query.Where("o.seller_id = ?", userID)
	}

	// 有状态参数时，继续通过 AND 增加状态条件。
	if status != "" {
		query = query.Where("o.status = ?", status)
	}

	// 初始化空切片，没有订单时返回 []。
	orders := make([]OrderResponse, 0)

	// 按编号从大到小执行查询，将结果装入响应切片。
	result := query.Order("o.id DESC").Scan(&orders)
	if result.Error != nil {
		log.Printf("查询订单列表失败：%v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询订单失败，请稍后重试"})
		return
	}

	// 返回当前用户对应角色下的订单列表。
	c.JSON(http.StatusOK, gin.H{
		"message": "查询成功",
		"data":    orders,
	})
}

// Detail 只允许订单买家或卖家查询订单详情。
func (h *OrderHandler) Detail(c *gin.Context) {
	// 读取当前登录用户编号。
	value, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "请先登录"})
		return
	}

	// 验证用户编号类型及范围。
	userID, ok := value.(int64)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "登录信息无效，请重新登录"})
		return
	}

	// 将路径中的订单编号转换为正整数。
	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || orderID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "订单编号必须是正整数"})
		return
	}

	// 同时限制订单编号和访问者身份，避免返回无权查看的订单。
	var order OrderResponse
	result := h.queryWithNames().
		Where(
			"o.id = ? AND (o.buyer_id = ? OR o.seller_id = ?)",
			orderID,
			userID,
			userID,
		).
		Take(&order)

	// 不存在和无权访问均返回 404，不透露他人的订单是否存在。
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在或无权查看"})
		return
	}

	// 其他数据库错误返回 500。
	if result.Error != nil {
		log.Printf("查询订单详情失败：%v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询订单失败，请稍后重试"})
		return
	}

	// 返回订单快照及买卖双方姓名。
	c.JSON(http.StatusOK, gin.H{
		"message": "查询成功",
		"data":    order,
	})
}

// Cancel 取消当前买家的待支付订单，并释放教材。
func (h *OrderHandler) Cancel(c *gin.Context) {
	// 读取认证中间件保存的用户编号。
	value, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "请先登录"})
		return
	}

	// 确认编号类型正确且大于零。
	userID, ok := value.(int64)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "登录信息无效，请重新登录",
		})
		return
	}

	// 路径中的 id 是订单编号，不是教材编号。
	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || orderID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "订单编号必须是正整数",
		})
		return
	}

	// 两次状态更新放入同一个事务，任何一步失败都会回滚。
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// 只查当前用户作为买家的订单。
		// 卖家或无关用户不能通过这个条件取得订单。
		var order models.Order
		if err := tx.
			Where("id = ? AND buyer_id = ?", orderID, userID).
			First(&order).Error; err != nil {
			return err
		}

		// 已经取消的订单直接成功，不再操作教材。
		// 避免旧订单的重复取消影响教材后来的交易。
		if order.Status == models.OrderStatusCancelled {
			return nil
		}

		// 已支付或已完成的订单，不允许通过取消接口处理。
		if order.Status != models.OrderStatusPending {
			return errOrderConflict
		}

		// 先更新教材，再更新订单，保持后续交易操作的加锁顺序一致。
		// 只有教材仍属于原卖家且处于 reserved 状态时，才能释放。
		bookResult := tx.Model(&models.Book{}).
			Where(
				"id = ? AND seller_id = ? AND status = ?",
				order.BookID,
				order.SellerID,
				"reserved",
			).
			Update("status", "on_sale")

		// SQL 执行失败时回滚。
		if bookResult.Error != nil {
			return bookResult.Error
		}

		// 没有匹配教材时，不能继续把订单标记为已取消。
		if bookResult.RowsAffected == 0 {
			return errOrderBookConflict
		}

		// 取得取消时间。
		now := time.Now()

		// 更新时再次检查订单仍为待支付，并且买家没有变化。
		// 不能只相信前面查询时看到的旧状态。
		orderResult := tx.Model(&models.Order{}).
			Where(
				"id = ? AND buyer_id = ? AND status = ?",
				orderID,
				userID,
				models.OrderStatusPending,
			).
			Updates(map[string]interface{}{
				"status":       models.OrderStatusCancelled,
				"cancelled_at": now,
			})

		// 更新失败时，前面释放教材的操作也会回滚。
		if orderResult.Error != nil {
			return orderResult.Error
		}

		// 状态已被其他请求改变，也必须回滚，不能留下错误释放的教材。
		if orderResult.RowsAffected == 0 {
			return errOrderConflict
		}

		// 两项操作均成功，提交事务。
		return nil
	})

	// 事务结束后，根据错误决定 HTTP 响应。
	if err != nil {
		// 订单不存在，或当前用户不是买家。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "订单不存在或无权取消",
			})
			return
		}

		// 业务状态不符合要求，返回 409。
		if errors.Is(err, errOrderConflict) ||
			errors.Is(err, errOrderBookConflict) {
			c.JSON(http.StatusConflict, gin.H{
				"message": "订单或教材状态已变化，请刷新后重试",
			})
			return
		}

		// 其他数据库错误记录到后端日志。
		log.Printf("取消订单失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "取消订单失败，请稍后重试",
		})
		return
	}

	// 只有事务成功，或订单原本已经取消，才返回成功。
	c.JSON(http.StatusOK, gin.H{
		"message": "订单已取消",
		"data": gin.H{
			"id":     orderID,
			"status": models.OrderStatusCancelled,
		},
	})
}
