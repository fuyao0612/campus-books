# 支付宝沙箱支付模块

本模块实现课程项目的统一沙箱商家收款，不包含真实商户上线、分账结算、退款或平台抽成。买卖双方是系统用户，实际沙箱款项统一进入配置的支付宝沙箱商家账号，并不会自动转给教材发布人。

## 文件阅读顺序

1. `../models/order.go`：订单、付款时间和固定付款窗口。
2. `../models/payment.go`：核验通过的成功支付记录及唯一索引。
3. `../config/alipay.go`：读取本机配置和密钥。
4. `alipay.go`：SDK 初始化、沙箱限制、公钥误用检查、HTTP 超时。
5. `amount.go`：整数分与元字符串转换，避免浮点金额误差。
6. `gateway.go`：SDK 的发链、主动查单和关闭交易封装。
7. `service.go`：事务、锁、重复通知、取消与支付状态协调。
8. `../handlers/payment.go`：HTTP 参数、权限、通知验签和响应。

## 配置

在项目根目录 `.env` 配置：

```dotenv
ALIPAY_SANDBOX=true
ALIPAY_APP_ID=沙箱应用APPID
ALIPAY_SELLER_ID=沙箱商家支付宝用户ID
ALIPAY_APP_PRIVATE_KEY_FILE=secrets/alipay-app-private.pem
ALIPAY_PUBLIC_KEY_FILE=secrets/alipay-public.pem
ALIPAY_RETURN_BASE_URL=http://127.0.0.1:5174/payment.html
ALIPAY_NOTIFY_URL=https://你的实际公网域名/api/payments/alipay/notify
```

应用公钥配置到控制台，应用私钥保存在本机；`alipay-public.pem` 必须保存控制台的**支付宝公钥**，不能放应用公钥。两者误用会在启动时明确报错。密钥文件和 `.env` 都不要提交到 Git。

通知地址允许在准备阶段留空以启动其他功能；实际发起支付必须是公网 HTTPS、固定通知路径，不允许 localhost、内网IP、账号信息或片段。地址检查不能保证隧道在线，需实际访问公网健康检查验证。

临时隧道保持运行：

```powershell
& 'C:\Program Files (x86)\cloudflared\cloudflared.exe' tunnel --url http://127.0.0.1:8080
```

隧道重启后可能换域名；更新 `.env` 并重启后端，旧链接中的回调地址不会自动改变。浏览器返回地址访问的是前端 5174，支付宝通知访问的是后端 8080，不能混淆。

## 接口

除通知外均需要 `Authorization: Bearer <token>`；操作接口的请求体可为 `{}`。

| 方法 | 路径 | 权限与作用 |
| --- | --- | --- |
| POST | `/api/orders/:id/pay` | 买家，待支付订单；返回 `data.payment_url` |
| POST | `/api/orders/:id/sync` | 买卖双方，向支付宝主动查单并同步核验结果 |
| POST | `/api/orders/:id/cancel` | 买家，先协调平台关闭，再取消并释放教材 |
| POST | `/api/orders/:id/complete` | 买家，确认已支付订单收货；可重复调用 |
| POST | `/api/payments/alipay/notify` | 无JWT，RSA2签名及应用/收款账号校验 |

通知使用 `application/x-www-form-urlencoded`，成功处理回复纯文本 `success`；解析、验签或入账失败不回复成功。待付款、关闭通知只确认收到，不自动退款或将已付款订单取消。

## 状态与一致性

```text
下单：教材 on_sale → reserved；订单创建为 pending
付款：教材 reserved → sold；订单 pending → paid；创建唯一成功支付记录
取消：教材 reserved → on_sale；订单 pending → cancelled
收货：教材保持 sold；订单 paid → completed
```

- 每次状态变更都在数据库事务中执行。通知和主动查单共用同一入账函数。
- SQL Server 使用 `UPDLOCK, ROWLOCK`，统一先锁教材、再锁订单；不使用该数据库不支持的 `FOR UPDATE`。
- 金额从订单快照读取，客户端不能决定应付金额。回调核对应用、收款账号、通知类型、业务订单号、平台流水、金额和付款状态。
- 同一订单和同一平台流水分别有唯一索引。重复通知必须与原有记录逐项匹配，且不能把 completed 改回 paid。
- 付款日期按中国时区解析；收货、付款和取消时间分别保存。重复操作不会改写已有完成时间。
- 付款返回页会主动查单，刷新按钮也会调用 sync；不会因为浏览器地址包含 success 等参数而标记付款。

## 付款链接与取消

第一次发出付款链接前，保存 `payment_started_at` 与 `payment_expires_at`。付款链接固定15分钟有效，所有重试使用相同商户订单号和截止时间，不能重试延长窗口。

从未发出链接的订单可以直接在本地取消。已经发链的订单先请求支付宝关闭交易；关闭失败则查单：

- 已支付：核对后入账，取消接口返回409，提示刷新，教材保持售出。
- 已关闭：取消本地待支付订单并释放教材。
- 超时、签名错误或未知状态：不释放教材，可重试或主动查单恢复。
- 平台尚无交易：可能是用户尚未打开付款链接。链接仍有效时不释放教材；打开付款页建立交易后可再取消，或等固定截止时间过去并额外等待一分钟后重试取消。届时仍需平台查单确认交易不存在。

最后一项依赖支付宝执行请求中的绝对 `time_expire`。不要将“平台交易不存在”直接等同于“可以立即释放教材”，否则旧链接可能在取消后又发起付款。所有超时采用保留占用的处理，避免错误重复出售。

本版不含定时清理任务。未付款占用由用户取消或主动查询已关闭交易释放；不要直接在数据库中清空付款发起标记或手工恢复在售。

## 启动与实际付款验收

```powershell
cd E:\campus-books
go run .
```

首次启动会给订单增加两个可空的付款时间字段，保留旧数据。另开终端启动前端，若旧前端服务仍在运行，先停止后重新启动以加载30秒代理超时：

```powershell
node web/server.mjs
```

使用沙箱买家账号测试，不能用收款的沙箱商家账号给自己付款：

1. 卖家发布低价测试教材，买家登录后创建订单。
2. 打开订单付款页，点击前往支付，用沙箱买家账号付款。
3. 保持 Go 服务与隧道运行，付款后返回前端，点击刷新状态。
4. 核对订单为 paid、教材为 sold、成功支付记录只有一条，金额与订单一致。
5. 买家确认收货，订单为 completed，重复通知不应退回 paid。
6. 另建订单测试取消、重复取消、旧取消不能释放新订单占用，以及无关账号不能付款或收货。

付款失败或网络异常时先查单，不要手工把订单改成 paid。未实现退款，不要在沙箱之外处理实际款项。

## 自动验证

```powershell
go test ./... -count=1
go vet ./...
```

常规测试使用临时生成的独立应用/平台 RSA 密钥、真实 SDK 请求编码和响应验签、本机 HTTP 服务，以及 SQL Server 方言的 SQL mock。覆盖金额边界、通知篡改、错误应用/收款方、重复通知、入账回滚、重复发链、权限、平台失败、取消与付款协调和收货。

显式运行真实沙箱**只读**查单，不创建交易、不付款：

```powershell
$env:CAMPUS_ALIPAY_PROBE = '1'
go test ./payments -run TestSandboxReadOnlyProbe -v -count=1
Remove-Item Env:CAMPUS_ALIPAY_PROBE
```

独立 SQL Server 集成测试需要当前数据库账号拥有建库权限，测试只创建并清理 `campus_payment_test_数字` 临时数据库，不修改项目库：

```powershell
$env:CAMPUS_PAYMENT_SQL_TEST = '1'
go test ./payments -run TestSQLServerPaymentIntegration -v -count=1
Remove-Item Env:CAMPUS_PAYMENT_SQL_TEST
```

无建库权限或无法创建隔离数据库时会明确跳过，不能把 skip 当成并发验证通过。常规 SQL mock 验证事务调用和回滚分支，不能证明真实数据库的锁调度。

2026-11-16：常规测试、真实 SDK 的签名HTTP模拟、通知验签及重放检查通过；更正公钥后真实沙箱只读查单通过。真实 SQL Server 隔离并发测试因无法创建测试库而跳过。尚未由自动化工具执行完整沙箱付款和真实商户上线验收。
