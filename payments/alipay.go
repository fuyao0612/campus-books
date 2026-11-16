package payments

import (
	"fmt" // 包装初始化失败的错误。
	"net/http"
	"time"

	"campus-books/config" // 使用已经写好的支付宝配置结构体。

	"github.com/smartwalle/alipay/v3" // 支付宝第三方 Go SDK。
	"github.com/smartwalle/ncrypto"
)

// NewAlipay 创建并初始化支付宝沙箱客户端。
// 成功返回客户端；失败返回错误，不继续执行支付操作。
func NewAlipay(cfg *config.AlipayConfig) (*alipay.Client, error) {
	// 防止调用者意外传入空指针，导致后面访问字段时 panic。
	if cfg == nil {
		return nil, fmt.Errorf("支付宝配置不能为空")
	}

	// 当前项目只接沙箱，再次确认不使用真实支付环境。
	if !cfg.Sandbox {
		return nil, fmt.Errorf("当前仅允许初始化支付宝沙箱客户端")
	}

	// SDK 的第三个参数叫 production，表示是否使用正式环境。
	// false 表示沙箱，不能直接把 cfg.Sandbox 传进去。
	//
	// 第一个参数：应用 APPID。
	// 第二个参数：应用私钥的内容，不是文件路径。
	// SDK 会解析私钥，并准备后续请求使用的签名器。
	client, err := alipay.New(
		cfg.AppID,
		cfg.AppPrivateKey,
		false,
	)
	if err != nil {
		// %w 保留原始错误，方便定位私钥格式等问题。
		return nil, fmt.Errorf("初始化支付宝客户端失败：%w", err)
	}

	// 加载支付宝公钥，为后续验证平台签名做准备。
	// 这里传入的也是公钥内容，不是文件路径。
	if err := client.LoadAliPayPublicKey(cfg.AlipayPublicKey); err != nil {
		return nil, fmt.Errorf("加载支付宝公钥失败：%w", err)
	}

	// 应用公钥和支付宝公钥属于不同的签名方，不能混用。
	privateKey, parseErr := ncrypto.DecodePrivateKey([]byte(cfg.AppPrivateKey)).PKCS1().RSAPrivateKey()
	if parseErr != nil {
		privateKey, parseErr = ncrypto.DecodePrivateKey([]byte(cfg.AppPrivateKey)).PKCS8().RSAPrivateKey()
	}
	publicKey, publicErr := ncrypto.DecodePublicKey([]byte(cfg.AlipayPublicKey)).PKIX().RSAPublicKey()
	if parseErr == nil && publicErr == nil && privateKey.PublicKey.Equal(publicKey) {
		return nil, fmt.Errorf("支付宝公钥误用了应用公钥，请从沙箱控制台取得支付宝公钥后替换本机文件")
	}
	// 关闭交易和主动查单都需要 HTTP 请求，设置超时避免一直阻塞。
	client.Client = &http.Client{Timeout: 8 * time.Second}

	// 私钥、公钥均能解析，返回客户端。
	return client, nil
}
