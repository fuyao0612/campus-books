package payments

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateNotifyURL 在实际发起支付时要求填写公网 HTTPS 回调。
// 这里只校验地址，公网转发是否保持在线仍需实际联调确认。
func ValidateNotifyURL(address string) error {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.Path != "/api/payments/alipay/notify" {
		return fmt.Errorf("请配置公网 HTTPS 通知地址，路径为 /api/payments/alipay/notify")
	}
	host := strings.ToLower(u.Hostname())
	ip := net.ParseIP(host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || (ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast())) {
		return fmt.Errorf("支付宝通知地址不能使用本机或内网地址")
	}
	return nil
}
