package config

import (
	"fmt"     // 构造带上下文的错误信息。
	"net/url" // 解析并检查返回地址和通知地址。
	"os"      // 读取环境变量和本地文件。
	"strconv" // 将 true/false 字符串转换为布尔值。
	"strings" // 清理配置两端空白。
)

// AlipayConfig 保存支付宝初始化需要的配置。
// 字段没有 JSON 标签，也不应通过接口返回给前端。
type AlipayConfig struct {
	Sandbox         bool   // 是否使用沙箱。
	AppID           string // 支付宝应用编号。
	AppPrivateKey   string // 从文件读取的应用私钥。
	AlipayPublicKey string // 从文件读取的支付宝公钥。
	ReturnBaseURL   string // 浏览器付款后的返回页面。
	NotifyURL       string // 支付宝向后端发送通知的地址。
	// 沙箱商家的支付宝用户 ID，用于核对通知中的收款方。
	SellerID string
}

// LoadAlipay 从环境变量和文件加载支付宝配置。
// 成功返回配置和 nil；失败返回 nil 和具体错误。
func LoadAlipay() (*AlipayConfig, error) {
	// ParseBool 把字符串转换为 bool。
	// 缺失或拼错会返回错误，不会悄悄切换环境。
	sandbox, err := strconv.ParseBool(
		strings.TrimSpace(os.Getenv("ALIPAY_SANDBOX")),
	)
	if err != nil {
		return nil, fmt.Errorf("ALIPAY_SANDBOX 必须填写 true 或 false")
	}

	// 当前阶段只接沙箱，避免配置错误时使用真实支付环境。
	if !sandbox {
		return nil, fmt.Errorf("当前项目仅支持支付宝沙箱，请设置 ALIPAY_SANDBOX=true")
	}

	// 创建配置对象，并清理各项配置两端的空白。
	cfg := &AlipayConfig{
		Sandbox:       sandbox,
		AppID:         strings.TrimSpace(os.Getenv("ALIPAY_APP_ID")),
		ReturnBaseURL: strings.TrimSpace(os.Getenv("ALIPAY_RETURN_BASE_URL")),
		NotifyURL:     strings.TrimSpace(os.Getenv("ALIPAY_NOTIFY_URL")),
		SellerID:      strings.TrimSpace(os.Getenv("ALIPAY_SELLER_ID")),
	}

	// APPID 缺失时直接返回错误，不继续初始化。
	if cfg.AppID == "" {
		return nil, fmt.Errorf("请配置 ALIPAY_APP_ID")
	}

	// 收款方必须明确配置，不能只验证应用编号。
	if cfg.SellerID == "" {
		return nil, fmt.Errorf("请配置 ALIPAY_SELLER_ID")
	}

	// 同一段地址校验逻辑分别用于返回地址与通知地址。
	// map 的键是配置名，值是该配置对应的地址。
	for name, address := range map[string]string{
		"ALIPAY_RETURN_BASE_URL": cfg.ReturnBaseURL,
		"ALIPAY_NOTIFY_URL":      cfg.NotifyURL,
	} {
		// 当前准备阶段允许通知地址为空。
		// continue 只跳过本轮校验，不会结束整个 LoadAlipay 函数。
		// 如果已经填写通知地址，仍然必须通过下面的格式检查。
		if name == "ALIPAY_NOTIFY_URL" && address == "" {
			continue
		}
		// url.Parse 解析地址，但它本身也允许相对路径，
		// 所以下面还要检查协议和主机名。
		parsed, err := url.Parse(address)

		// || 从左到右短路判断：解析失败后不会访问 parsed。
		if err != nil ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Hostname() == "" ||
			parsed.User != nil ||
			parsed.Fragment != "" {
			return nil, fmt.Errorf("%s 必须是有效的 HTTP/HTTPS 绝对地址，且不能包含账号或片段", name)
		}
	}

	// 读取两个密钥文件的路径。
	privatePath := strings.TrimSpace(os.Getenv("ALIPAY_APP_PRIVATE_KEY_FILE"))
	publicPath := strings.TrimSpace(os.Getenv("ALIPAY_PUBLIC_KEY_FILE"))

	// 文件路径不可缺失。
	if privatePath == "" || publicPath == "" {
		return nil, fmt.Errorf("请配置应用私钥文件路径和支付宝公钥文件路径")
	}

	// ReadFile 读取文件，返回字节切片和错误。
	// 相对路径以启动程序时的工作目录为基准。
	privateData, err := os.ReadFile(privatePath)
	if err != nil {
		// %w 包装原始错误，保留“文件不存在”等具体原因。
		// 此处没有输出文件内容。
		return nil, fmt.Errorf("读取应用私钥文件失败：%w", err)
	}

	// 读取支付宝公钥文件。
	publicData, err := os.ReadFile(publicPath)
	if err != nil {
		return nil, fmt.Errorf("读取支付宝公钥文件失败：%w", err)
	}

	// 将字节切片转成字符串，并去掉文件两端空白。
	cfg.AppPrivateKey = strings.TrimSpace(string(privateData))
	cfg.AlipayPublicKey = strings.TrimSpace(string(publicData))

	// 能打开文件不代表里面有内容，因此额外检查空文件。
	if cfg.AppPrivateKey == "" || cfg.AlipayPublicKey == "" {
		return nil, fmt.Errorf("应用私钥文件和支付宝公钥文件不能为空")
	}

	// 返回配置对象；nil 表示加载成功。
	return cfg, nil
}
