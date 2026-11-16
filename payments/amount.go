package payments

import (
	"fmt"     // 格式化金额，以及构造错误。
	"strconv" // 把数字字符串转换为整数。
	"strings" // 拆分金额中的整数部分和小数部分。
)

// CentsToYuan 将整数分转换为支付宝使用的元字符串。
// 例如：1999 → "19.99"，1 → "0.01"。
func CentsToYuan(cents int64) (string, error) {
	// 支付金额必须大于零。
	if cents <= 0 {
		return "", fmt.Errorf("支付金额必须大于零")
	}

	// SDK 文档中订单总金额上限为一亿元，即一百亿分。
	if cents > 10_000_000_000 {
		return "", fmt.Errorf("支付金额超过允许范围")
	}

	// 整数除法取得“元”。
	// 例如 1999 / 100 得到 19。
	yuan := cents / 100

	// 取余取得不足一元的“分”。
	// 例如 1999 % 100 得到 99。
	fraction := cents % 100

	// %d 输出整数。
	// %02d 输出至少两位整数，不足两位时在左边补零。
	// 例如 yuan=25、fraction=5，得到 "25.05"。
	return fmt.Sprintf("%d.%02d", yuan, fraction), nil
}

// YuanToCents 将支付宝返回的元字符串转换为整数分。
// 支持 "25"、"25.0"、"25.00"，拒绝负数、科学计数法和超过两位小数。
func YuanToCents(amount string) (int64, error) {
	// 按小数点拆开，例如 "19.99" 得到 ["19", "99"]。
	parts := strings.Split(amount, ".")

	// 金额不能为空，且最多只能有一个小数点。
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("金额格式不正确")
	}

	// 只允许数字和小数点，不接受空格、正负号或指数。
	for _, char := range amount {
		if (char < '0' || char > '9') && char != '.' {
			return 0, fmt.Errorf("金额只能包含数字和小数点")
		}
	}

	// 将整数部分按十进制转换为 int64。
	yuan, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("金额整数部分无效：%w", err)
	}

	// 先限制元的范围，避免后面乘以 100 时发生整数溢出。
	if yuan > 100_000_000 {
		return 0, fmt.Errorf("金额超过允许范围")
	}

	// 默认小数部分为零。
	var fraction int64

	// 如果存在小数点，就检查并解析小数部分。
	if len(parts) == 2 {
		decimal := parts[1]

		// 不接受 "25."，也不接受 "25.001"。
		if len(decimal) == 0 || len(decimal) > 2 {
			return 0, fmt.Errorf("金额小数部分必须为一位或两位")
		}

		// 一位小数要在右边补零："25.5" 表示 25 元 50 分。
		if len(decimal) == 1 {
			decimal += "0"
		}

		// 前面已检查字符，这里继续检查转换是否成功。
		fraction, err = strconv.ParseInt(decimal, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("金额小数部分无效：%w", err)
		}
	}

	// 使用整数运算组装总分数。
	cents := yuan*100 + fraction

	// 检查金额下限，以及一亿元边界上的小数情况。
	if cents <= 0 || cents > 10_000_000_000 {
		return 0, fmt.Errorf("金额不在允许范围内")
	}

	return cents, nil
}
