package payments

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"campus-books/config"
	"github.com/joho/godotenv"
	"github.com/smartwalle/ncrypto"
)

// 显式启用：仅查询一个不存在的随机商户订单，不创建交易、不付款、不改数据库。
func TestSandboxReadOnlyProbe(t *testing.T) {
	if os.Getenv("CAMPUS_ALIPAY_PROBE") != "1" {
		t.Skip("显式启用才访问真实沙箱")
	}
	values, err := godotenv.Read("../.env")
	if err != nil {
		t.Fatal("无法读取本地配置")
	}
	for name, value := range values {
		if strings.HasPrefix(name, "ALIPAY_") {
			t.Setenv(name, value)
		}
	}
	// .env 内的密钥路径相对于项目根目录；测试进程位于 payments 目录。
	for _, name := range []string{"ALIPAY_APP_PRIVATE_KEY_FILE", "ALIPAY_PUBLIC_KEY_FILE"} {
		value := os.Getenv(name)
		if !strings.Contains(value, ":") && !strings.HasPrefix(value, "/") {
			t.Setenv(name, "../"+value)
		}
	}
	cfg, err := config.LoadAlipay()
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewAlipay(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// 只比较公钥数学值，不输出任何密钥字节。
	private, keyErr := ncrypto.DecodePrivateKey([]byte(cfg.AppPrivateKey)).PKCS1().RSAPrivateKey()
	if keyErr != nil {
		private, keyErr = ncrypto.DecodePrivateKey([]byte(cfg.AppPrivateKey)).PKCS8().RSAPrivateKey()
	}
	public, publicErr := ncrypto.DecodePublicKey([]byte(cfg.AlipayPublicKey)).PKIX().RSAPublicKey()
	if keyErr == nil && publicErr == nil && private.PublicKey.Equal(public) {
		t.Fatal("支付宝公钥与应用私钥对应的公钥相同：很可能误填了应用公钥，请从沙箱控制台重新取得支付宝公钥")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = (SDKGateway{Client: client}).Query(ctx, fmt.Sprintf("probe_%d", time.Now().UnixNano()))
	if !errors.Is(err, ErrTradeMissing) {
		t.Fatalf("沙箱只读查单未得到预期的交易不存在结果：%v", err)
	}
	t.Log("沙箱只读查单返回交易不存在；未创建交易、未付款、未访问数据库")
}
