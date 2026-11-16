package handlers

import (
	"context"
	"errors"
	"log"
	"mime"
	"net/http"
	"strconv"
	"time"

	"campus-books/config"
	"campus-books/payments"
	"github.com/gin-gonic/gin"
	"github.com/smartwalle/alipay/v3"
	"gorm.io/gorm"
)

// PaymentHandler 保留 SDK 负责验签，Service 负责所有支付和订单状态事务。
type PaymentHandler struct {
	DB      *gorm.DB
	Client  *alipay.Client
	Config  *config.AlipayConfig
	Service *payments.Service
}

// paymentIdentity 集中读取已验证身份和路径编号，不接受请求体中的 buyer_id。
func paymentIdentity(c *gin.Context) (int64, int64, bool) {
	value, exists := c.Get("user_id")
	userID, ok := value.(int64)
	if !exists || !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "请先登录"})
		return 0, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "订单编号必须是正整数"})
		return 0, 0, false
	}
	return userID, id, true
}
func paymentError(c *gin.Context, err error) {
	code, message := http.StatusInternalServerError, "支付操作失败，请稍后重试"
	switch {
	case errors.Is(err, payments.ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		code, message = http.StatusNotFound, "订单不存在或无权操作"
	case errors.Is(err, payments.ErrConflict), errors.Is(err, payments.ErrAlreadyPaid), errors.Is(err, payments.ErrAwaitExpiry):
		code, message = http.StatusConflict, err.Error()
	case errors.Is(err, payments.ErrInvalidPayment):
		code, message = http.StatusConflict, "支付数据与订单不一致，请联系维护人"
	case errors.Is(err, payments.ErrGateway):
		code, message = http.StatusBadGateway, "支付平台暂不可用，请稍后重试"
	default:
		log.Printf("支付操作失败：%v", err)
	}
	c.JSON(code, gin.H{"message": message})
}
func (h *PaymentHandler) Pay(c *gin.Context) {
	userID, id, ok := paymentIdentity(c)
	if !ok {
		return
	}
	if err := payments.ValidateNotifyURL(h.Config.NotifyURL); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "支付通知地址尚未配置完成，请联系维护人"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	address, err := h.Service.Pay(ctx, id, userID)
	if err != nil {
		paymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "付款地址已生成", "data": gin.H{"payment_url": address}})
}
func (h *PaymentHandler) Cancel(c *gin.Context) {
	userID, id, ok := paymentIdentity(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	order, err := h.Service.Cancel(ctx, id, userID)
	if err != nil {
		paymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "订单已取消", "data": order})
}
func (h *PaymentHandler) Sync(c *gin.Context) {
	userID, id, ok := paymentIdentity(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	order, err := h.Service.Sync(ctx, id, userID)
	if err != nil {
		paymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "订单状态已刷新", "data": order})
}
func (h *PaymentHandler) Complete(c *gin.Context) {
	userID, id, ok := paymentIdentity(c)
	if !ok {
		return
	}
	order, err := h.Service.Complete(c.Request.Context(), id, userID)
	if err != nil {
		paymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已确认收到教材", "data": order})
}

// Notify 接收支付宝表单；不使用JWT，平台签名才是通知身份凭据。
func (h *PaymentHandler) Notify(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/x-www-form-urlencoded" {
		c.String(415, "failure")
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		c.String(400, "failure")
		return
	}
	values := c.Request.PostForm // 不混入URL查询参数。
	for _, items := range values {
		if len(items) != 1 {
			c.String(400, "failure")
			return
		}
	}
	if values.Get("sign") == "" || values.Get("sign_type") != "RSA2" {
		c.String(400, "failure")
		return
	}
	n, err := h.Client.DecodeNotification(c.Request.Context(), values)
	if err != nil {
		log.Println("支付宝通知验签失败")
		c.String(400, "failure")
		return
	}
	if n.AppId != h.Config.AppID || n.SellerId != h.Config.SellerID || n.NotifyType != alipay.NotifyTypeTradeStatusSync {
		c.String(400, "failure")
		return
	}
	switch n.TradeStatus {
	case alipay.TradeStatusWaitBuyerPay, alipay.TradeStatusClosed:
		// 不把关闭通知当成退款或取消指令。主动查单/取消接口负责释放未支付订单。
		c.String(200, "success")
		return
	case alipay.TradeStatusSuccess, alipay.TradeStatusFinished:
	default:
		c.String(400, "failure")
		return
	}
	if n.OutTradeNo == "" || len(n.OutTradeNo) > 64 || n.TradeNo == "" || len(n.TradeNo) > 64 {
		c.String(400, "failure")
		return
	}
	amount, err := payments.YuanToCents(n.TotalAmount)
	if err != nil {
		c.String(400, "failure")
		return
	}
	trade := &payments.TradeResult{Number: n.OutTradeNo, TradeNo: n.TradeNo, Amount: amount, Status: n.TradeStatus, PaidAt: n.GmtPayment}
	err = h.Service.AcceptPaid(c.Request.Context(), trade)
	if err != nil {
		// 不提前确认入账；平台可重试。日志不含完整表单、密钥或签名。
		log.Printf("支付宝通知入账未完成：%v", err)
		c.String(500, "failure")
		return
	}
	c.String(200, "success") // 只有事务提交或严格核对的重复通知才确认成功。
}
