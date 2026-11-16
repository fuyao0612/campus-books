package payments

import "testing" // Go 标准库提供的测试工具。

// TestCentsToYuan 检查整数分能否准确转换为元字符串。
// 测试函数名必须以 Test 开头，并接收 *testing.T。
func TestCentsToYuan(t *testing.T) {
	// 每一行是一组测试数据：
	// name 是用例名称，input 是输入，want 是预期结果。
	tests := []struct {
		name    string
		input   int64
		want    string
		wantErr bool // true 表示这组输入应当返回错误。
	}{
		{"一分钱", 1, "0.01", false},
		{"小数补零", 2505, "25.05", false},
		{"普通金额", 1999, "19.99", false},
		{"整元", 2500, "25.00", false},
		{"金额上限", 10_000_000_000, "100000000.00", false},
		{"零金额", 0, "", true},
		{"负数", -1, "", true},
		{"超过上限", 10_000_000_001, "", true},
	}

	// 遍历测试数据，逐组调用被测试的函数。
	for _, tc := range tests {
		// t.Run 创建子测试，失败时能直接看到用例名称。
		t.Run(tc.name, func(t *testing.T) {
			got, err := CentsToYuan(tc.input)

			// 判断实际是否出错，与预期是否出错一致。
			if (err != nil) != tc.wantErr {
				t.Fatalf(
					"输入 %d：错误=%v，预期出错=%t",
					tc.input, err, tc.wantErr,
				)
			}

			// 对合法输入，继续比较转换后的金额字符串。
			if !tc.wantErr && got != tc.want {
				t.Errorf("得到 %q，期望 %q", got, tc.want)
			}
		})
	}
}

// TestYuanToCents 检查元字符串能否准确转换为整数分，
// 同时确认非法格式会被拒绝。
func TestYuanToCents(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"一分钱", "0.01", 1, false},
		{"整元", "25", 2500, false},
		{"一位小数", "25.5", 2550, false},
		{"两位小数", "19.99", 1999, false},
		{"小数前导零", "25.05", 2505, false},
		{"金额上限", "100000000.00", 10_000_000_000, false},
		{"空字符串", "", 0, true},
		{"零金额", "0.00", 0, true},
		{"负数", "-1.00", 0, true},
		{"带正号", "+1.00", 0, true},
		{"前后空格", " 1.00 ", 0, true},
		{"科学计数法", "1e2", 0, true},
		{"缺少整数", ".50", 0, true},
		{"缺少小数", "25.", 0, true},
		{"三位小数", "25.001", 0, true},
		{"多个小数点", "1.2.3", 0, true},
		{"超过金额上限", "100000000.01", 0, true},
		{"整数溢出", "9223372036854775808", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := YuanToCents(tc.input)

			// 先检查是否正确接受或拒绝这组输入。
			if (err != nil) != tc.wantErr {
				t.Fatalf(
					"输入 %q：错误=%v，预期出错=%t",
					tc.input, err, tc.wantErr,
				)
			}

			// 合法输入必须得到准确的整数分。
			if !tc.wantErr && got != tc.want {
				t.Errorf("得到 %d 分，期望 %d 分", got, tc.want)
			}
		})
	}
}
