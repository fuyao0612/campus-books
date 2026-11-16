# 接口说明

基础地址 `http://127.0.0.1:8080`。浏览器通过前端同源 `/api` 代理访问。成功通常返回 `{message,data}`，业务失败返回 `{message}`；支付通知返回纯文本。写接口使用JSON，支付宝通知使用表单。

需要登录的接口携带 `Authorization: Bearer <token>`。JWT包含user_id、姓名、签发者和两小时有效期，不包含密码。后端进一步检查资源归属和业务状态。

| 方法 | 路径 | 权限 | 输入或用途 |
| --- | --- | --- | --- |
| GET | /api/health | 公开 | 进程存活 |
| GET | /api/ready | 公开 | 数据库就绪 |
| POST | /api/login | 公开 | `{name,password}`，返回token与用户 |
| GET | /api/me | 登录 | 当前用户 |
| GET | /api/books?keyword= | 公开 | 在售教材，书名或课程搜索 |
| GET | /api/books/:id | 公开 | 在售教材详情 |
| POST | /api/books | 登录 | 发布教材 |
| GET | /api/me/books?status= | 登录 | 本人教材，可按on_sale/reserved/off_shelf/sold筛选 |
| PUT | /api/books/:id | 发布人 | 完整更新本人在售教材 |
| PATCH | /api/books/:id/status | 发布人 | `{status:"off_shelf"}`，交易中不能下架 |
| GET | /api/books/:id/comments | 公开 | 留言倒序，支持已下架教材 |
| POST | /api/books/:id/comments | 登录 | `{content}`，仅在售教材接受新留言 |
| DELETE | /api/comments/:id | 作者 | 删除本人留言 |
| POST | /api/orders | 登录 | `{book_id}`，不可购买本人教材 |
| GET | /api/orders?role=buy&status= | 交易方 | role=buy/sell，status可省略 |
| GET | /api/orders/:id | 交易方 | 订单详情及买卖双方姓名 |
| POST | /api/orders/:id/pay | 买家 | 返回沙箱payment_url |
| POST | /api/orders/:id/sync | 交易方 | 向支付宝查单并同步核验结果 |
| POST | /api/orders/:id/cancel | 买家 | 协调平台关闭后取消待支付订单 |
| POST | /api/orders/:id/complete | 买家 | 确认已付款订单收货 |
| POST | /api/payments/alipay/notify | 平台签名 | RSA2通知核验与入账 |

## 教材请求

```json
{"title":"Go程序设计","course":"程序设计","category":"计算机","condition":"九成新","price":1999,"description":"少量笔记","contact":"测试联系"}
```

书名/课程/联系最长100字符，成色最长50字符，描述最长1000字符。必填文本清理两端空白后不能为空。category允许计算机/公共课/语言/设计/其他，省略时为其他。价格为大于零的int64整数分，调用支付时还受支付宝金额上限约束。PUT使用完整请求，省略description会清空描述，省略category会设为其他。

响应包含教材模型字段，列表和详情另包含seller_name。教材下架保留记录，不物理删除。

## 留言与订单

留言内容最多500字符，拒绝空白。列表包含id、book_id、user_id、name、content、created_at，按created_at和id倒序。删除接口的id是留言编号，不是教材编号。

订单响应含id、number、book_id、buyer_id、seller_id、title、price、contact、status、created_at、updated_at，以及可空的paid_at、completed_at、cancelled_at、payment_started_at、payment_expires_at。列表和详情另关联buyer_name/seller_name。number为业务订单号，平台out_trade_no使用该字段；路径id为数据库主键。

## 状态码

200查询/更新成功，201新建成功，400参数错误，401凭据无效，403已识别用户但无权修改该资源，404不存在或不向无关用户透露订单，409状态冲突，502支付平台不可用，503通知配置或数据库就绪失败。服务器内部异常返回500。

订单详情、取消和支付对无关用户统一返回404。支付回调不使用用户JWT。回调返回success只表示已核验并按当前业务规则处理；不得根据浏览器页面或客户端自报金额直接将订单改为paid。
