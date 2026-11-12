# 校园书集前端

这是校园二手教材交易系统的真实接口前端。页面不包含样例账号、浏览器假数据或模拟支付；所有业务数据都通过 Go API 获取和保存。前端已覆盖教材浏览、发布与管理、留言、订单和支付跳转。Go 服务还没有实现的接口会显示 HTTP 错误，前端不会假装操作成功。

## 启动

需要 Node.js 18+。在 `E:\campus-books` 项目根目录运行：

```powershell
go run .
```

另开终端运行前端：

```powershell
node web/server.mjs
```

访问 **http://127.0.0.1:5174**。如果之前启动过旧版前端，先在旧终端按 Ctrl+C，再重新运行。不要双击 HTML 或使用 Live Server；本地 Node 服务会把 `/api/` 请求转发给 `http://127.0.0.1:8080`，无需给 Gin 添加 CORS。

端口被占用时可指定其他端口：

```powershell
$env:WEB_PORT = '5175'
node web/server.mjs
```

## 当前可用的后端接口

项目现有服务已经支持：

| 用途 | 方法与路径 | 请求/响应要点 |
| --- | --- | --- |
| 登录 | `POST /api/login` | 请求 `{name,password}`；响应 `data` 包含 `token` 和 `user` |
| 当前用户 | `GET /api/me` | `Authorization: Bearer <token>` |
| 教材广场 | `GET /api/books?keyword=` | 返回在售教材数组 |
| 教材详情 | `GET /api/books/:id` | 现有实现仅返回在售教材 |
| 发布教材 | `POST /api/books` | Bearer；价格为整数分；返回新教材 |

登录使用数据库中已经创建的账号。当前没有注册页面。若后端未运行、账号验证失败或路由尚不存在，页面会展示可读的错误并提供重试。

## 前后端接口约定

前端 API 封装位于 `api.js`。以下接口是完整页面所需的 REST 契约；现有 Go 服务尚未实现的部分需要后续逐项添加。成功响应建议统一为 `{ "message": "...", "data": ... }`，失败响应使用合适的 HTTP 状态码和 `{ "message": "..." }`。

| 功能 | 方法与路径 | 请求数据 / 返回数据 |
| --- | --- | --- |
| 查询本人发布 | `GET /api/me/books?status=on_sale` | 状态可省略或为 `on_sale`、`off_shelf`、`sold`；返回教材数组 |
| 编辑本人教材 | `PUT /api/books/:id` | `{title,course,condition,price,description,contact}`；`price` 为整数分 |
| 下架本人教材 | `PATCH /api/books/:id/status` | `{status:"off_shelf"}`；服务端检查所有权与未完成交易 |
| 查询教材留言 | `GET /api/books/:id/comments` | 返回留言数组，字段含 `id,user_id,name,content,created_at` |
| 发布留言 | `POST /api/books/:id/comments` | `{content}`；需要登录 |
| 删除本人留言 | `DELETE /api/comments/:id` | 需要登录并由服务端校验留言归属 |
| 创建订单 | `POST /api/orders` | `{book_id}`；服务端重新读取价格并确保同一教材不会并发卖给多人 |
| 查询订单 | `GET /api/orders?role=buy&status=pending` | `role` 为 `buy` 或 `sell`，`status` 可省略；返回订单数组 |
| 订单详情 | `GET /api/orders/:id` | 仅买家或卖家可查询 |
| 取消待支付订单 | `POST /api/orders/:id/cancel` | 空 JSON `{}`；仅买家可取消待支付订单 |
| 创建支付 | `POST /api/orders/:id/pay` | 空 JSON `{}`；返回 `{payment_url}`，由真实支付服务生成 |
| 确认收货 | `POST /api/orders/:id/complete` | 空 JSON `{}`；仅买家可确认已支付订单 |

教材结构使用现有 Go 字段：`id,title,course,condition,price,description,contact,seller_id,status,created_at,updated_at`。另外发布和编辑会提交 `category`（计算机、公共课、语言、设计、其他）；现有 Go 模型尚未保存这个字段，后续需要补充，缺省时页面按标题与课程推断分类。`price` 始终是整数分。订单至少需要 `id,number,book_id,title,price,buyer_id,seller_id,status,created_at`，建议同时返回 `seller_name,contact` 以展示交接信息。订单状态建议为 `pending,paid,completed,cancelled`；教材状态为 `on_sale,off_shelf,sold`。

支付前端只负责请求创建支付并跳转到服务端返回的 `payment_url`。只有支付平台的服务端回调验证成功后，Go 后端才能把订单设为 `paid` 并把教材设为 `sold`。浏览器回跳不能作为付款成功依据。页面不保存银行卡信息，也不提供虚假的支付成功状态。

## 页面与代码

页面路由：`#/` 教材广场、`#/book/:id` 详情与留言、`#/publish` 发布、`#/edit/:id` 修改、`#/mine` 我的发布、`#/orders` 买入/卖出订单、`#/pay/:id` 支付、`#/profile` 个人中心、`#/guide` 使用指南。没有预约功能。

| 文件 | 用途 |
| --- | --- |
| `index.html` | 页面外壳、导航、登录与确认弹窗 |
| `style.css` | 桌面与移动端样式 |
| `app.js` | 路由、页面渲染、表单校验与用户操作 |
| `api.js` | JWT 会话和所有 Go API 请求 |
| `server.mjs` | 本地静态服务与 `/api/` 代理 |

JWT 保存在 `sessionStorage`，关闭浏览器标签后清除；刷新时会调用 `/api/me` 校验。服务端仍须对每个写操作检查身份、资源归属、教材/订单状态和并发条件。图片上传、在线注册和正式支付渠道可以在后续功能迭代中增加。
