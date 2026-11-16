package payments

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"campus-books/models"
	"github.com/smartwalle/alipay/v3"
)

// Gateway 把 SDK 边界集中起来，测试可以使用独立 HTTP 服务替代支付宝。
// 真实运行仍使用 SDK 的签名与验签，不提供跳过验签的开关。
type Gateway interface {
	PagePay(alipay.TradePagePay) (*url.URL, error)
	Query(context.Context, string) (*TradeResult, error)
	Close(context.Context, string) error
}

type TradeResult struct {
	Number  string
	TradeNo string
	Amount  int64
	Status  alipay.TradeStatus
	PaidAt  string
}

var (
	ErrTradeMissing   = errors.New("支付宝交易尚未创建")
	ErrGateway        = errors.New("支付平台暂不可用，请稍后重试")
	ErrInvalidPayment = errors.New("支付数据与订单不一致")
	ErrConflict       = errors.New("订单或教材状态已变化，请刷新后重试")
	ErrNotFound       = errors.New("订单不存在或无权操作")
	ErrAlreadyPaid    = errors.New("订单已付款，不能取消；已同步付款状态")
	ErrAwaitExpiry    = errors.New("付款链接仍有效且平台尚无可关闭交易，请在付款页创建交易后重试，或在链接到期后再取消")
)

// SDKGateway 只信任 SDK 验签通过且业务 code 成功的响应。
type SDKGateway struct{ Client *alipay.Client }

func (g SDKGateway) PagePay(p alipay.TradePagePay) (*url.URL, error) {
	return g.Client.TradePagePay(p)
}
func (g SDKGateway) Query(ctx context.Context, number string) (*TradeResult, error) {
	rsp, err := g.Client.TradeQuery(ctx, alipay.TradeQuery{OutTradeNo: number})
	if err != nil {
		var apiErr *alipay.Error
		if errors.As(err, &apiErr) && apiErr.SubCode == "ACQ.TRADE_NOT_EXIST" {
			return nil, ErrTradeMissing
		}
		return nil, fmt.Errorf("%w: %v", ErrGateway, err)
	}
	if rsp == nil {
		return nil, ErrGateway
	}
	if rsp.IsFailure() {
		if rsp.SubCode == "ACQ.TRADE_NOT_EXIST" {
			return nil, ErrTradeMissing
		}
		return nil, fmt.Errorf("%w: %s", ErrGateway, rsp.SubCode)
	}
	if rsp.OutTradeNo != number || rsp.TradeNo == "" {
		return nil, ErrInvalidPayment
	}
	amount, err := YuanToCents(rsp.TotalAmount)
	if err != nil {
		return nil, ErrInvalidPayment
	}
	return &TradeResult{Number: rsp.OutTradeNo, TradeNo: rsp.TradeNo, Amount: amount, Status: rsp.TradeStatus, PaidAt: rsp.SendPayDate}, nil
}
func (g SDKGateway) Close(ctx context.Context, number string) error {
	rsp, err := g.Client.TradeClose(ctx, alipay.TradeClose{OutTradeNo: number})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGateway, err)
	}
	if rsp == nil || rsp.IsFailure() {
		return ErrGateway
	}
	if rsp.OutTradeNo != number {
		return ErrInvalidPayment
	}
	return nil
}

// buyerOnly=false 允许买卖双方查单；付款、取消和收货只允许买家。
func canAccess(order models.Order, userID int64, buyerOnly bool) bool {
	return userID > 0 && (order.BuyerID == userID || (!buyerOnly && order.SellerID == userID))
}
