# 校园书集前端

前端使用原生 HTML、CSS、JavaScript 和 Node.js 本地静态服务，无需 npm 安装或打包。保持绿色、纸张和书籍的既有视觉风格，适配桌面和手机。

2026-11-16 已完成计划中的主要页面与交互。业务数据始终来自 Go API，页面中没有假账号、固定库存或模拟支付。订单、支付和删除留言的后端现已接入；付款结果由后端核验，前端不会伪造成功。支付接入规则见 ../payments/README.md。

## 启动

在项目根目录分别打开两个终端：

```powershell
cd E:\campus-books
go run .
```

```powershell
cd E:\campus-books
node web/server.mjs
```

访问 <http://127.0.0.1:5174/login.html>。`/` 也是首页入口，未登录时跳转登录页。不要直接双击 HTML 或使用其他静态服务；本地服务会将 `/api/` 转发到 `http://127.0.0.1:8080`。

前端端口可用 `$env:WEB_PORT = '5175'` 调整。修改 `server.mjs` 后需要重启 Node 服务，修改 HTML/CSS/JS 后刷新浏览器即可。

## 页面

所有地址都有实际 HTML 文件，可以直接访问和刷新，符合课程 login.html、index.html、detail.html 的页面命名要求。

| 地址 | 页面与交互 |
| --- | --- |
| `/login.html` | 姓名与密码登录、密码显示/隐藏、失败提示、登录后跳转 |
| `/index.html` | 书名/课程搜索、分类/成色筛选、价格排序、分页、重置筛选 |
| `/detail.html?id=12` | 教材详情、发布人/联系方式、留言列表、发布/删除本人留言、购买确认 |
| `/publish.html` | 发布教材、输入校验、元转整数分、文字封面预览 |
| `/edit.html?id=12` | 编辑本人在售教材、清空描述、保存失败保留输入 |
| `/mine.html` | 本人教材、在售/下架/售出筛选、编辑与下架确认、历史留言入口 |
| `/comments.html?id=12` | 教材历史留言，只读；适用于已下架教材 |
| `/orders.html` | 买入/卖出订单、状态筛选、去支付、取消、确认收货 |
| `/order.html?id=1` | 订单详情、教材交易快照、金额/联系信息/时间、操作与状态刷新 |
| `/payment.html?id=1` | 核对订单、请求支付地址、跳转付款 |
| `/payment.html?id=1&return=1` | 付款返回后的状态查询；仍以服务端订单状态为准 |
| `/profile.html` | 当前用户、业务入口、退出登录 |
| `/guide.html` | 使用指南、常见问题，无需登录 |

除登录页与使用指南外，前端页面要求先登录。后端目前仍允许匿名调用教材列表、详情与留言查询 API；页面守卫不能代替后端权限校验。旧版 `#/book/12` 等书签会迁移到对应 HTML 地址。

登录凭据保存在 `sessionStorage`，每次页面加载通过 `/api/me` 恢复身份；401 清除凭据并跳转登录，网络故障保留凭据并允许重试。退出会清除前端凭据并返回登录页，目前没有服务端令牌撤销功能。

## 已实现的后端接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/login` | `{name,password}`；返回 `data.token` 和 `data.user` |
| GET | `/api/me` | 当前用户，需要 Bearer Token |
| GET | `/api/books?keyword=` | 在售教材数组，关键词匹配书名或课程 |
| GET | `/api/books/:id` | 当前仅返回在售教材 |
| POST | `/api/books` | 发布，需要登录；价格为整数分 |
| GET | `/api/me/books?status=` | 本人教材，状态可省略或为 on_sale/reserved/off_shelf/sold |
| PUT | `/api/books/:id` | 编辑本人在售教材，返回至少 `{id}` |
| PATCH | `/api/books/:id/status` | `{status:"off_shelf"}`；本人下架 |
| GET | `/api/books/:id/comments` | 教材留言，含姓名，教材下架后仍可查询 |
| POST | `/api/books/:id/comments` | `{content}`；需要登录，最多500字符 |

成功响应以 `{message,data}` 包装，失败响应以 `{message}` 说明。未实现路由返回的文本 404 也有友好提示。任何新增接口仍须在后端验证身份、归属与业务状态。

## 订单、支付和删除接口

以下接口均已实现并接好前端调用。2026-11-16，用户确认已成功完成沙箱付款，项目订单状态正确。

| 方法 | 路径 | 请求与响应约定 |
| --- | --- | --- |
| DELETE | `/api/comments/:id` | 只允许本人删除，成功返回 200/204 |
| POST | `/api/orders` | 请求 `{book_id}`；服务端读取价格、校验归属及状态并防重复售出；返回至少 `{id}` |
| GET | `/api/orders?role=buy&status=pending` | role=buy/sell，status 可省略；只返回当前用户相关订单数组 |
| GET | `/api/orders/:id` | 仅交易相关方可读，返回订单对象 |
| POST | `/api/orders/:id/cancel` | `{}`；仅买家取消待支付订单 |
| POST | `/api/orders/:id/complete` | `{}`；仅买家确认已支付订单 |
| POST | `/api/orders/:id/pay` | `{}`；仅买家发起待支付订单付款，返回 `{payment_url}` |
| POST | `/api/orders/:id/sync` | `{}`；交易双方主动查询支付宝并同步订单，付款返回页及刷新按钮调用 |

订单对象约定：`id,number,book_id,title,price,buyer_id,seller_id,status,created_at`；补充 `seller_name,buyer_name,contact,paid_at` 可以完整展示交易信息。`price` 是订单快照金额，单位为分；不应随教材后续编辑改变。状态为 `pending,paid,completed,cancelled`。

支付地址应由服务端创建，前端仅接受 HTTP(S) URL。付款返回地址建议为 `/payment.html?id=订单编号&return=1`。即使 URL 中包含 success 等参数，前端也不会据此更新订单。只有服务端核验支付结果后，订单接口返回 paid/completed，页面才展示付款已确认。当前已接支付宝沙箱，使用固定15分钟付款窗口、平台关单、回调核验与事务入账。超时或状态不明时保留教材占用。

## 数据兼容说明

- 教材沿用现有字段 `id,title,course,condition,price,description,contact,seller_id,status,created_at,updated_at`。
- 教材列表和详情已返回 `seller_name`；用户记录缺失时显示编号，不伪造姓名。
- `category` 已持久化，前后端支持计算机、公共课、语言、设计、其他。
- 留言列表返回 `id,book_id,user_id,name,content,created_at`；发布响应可直接使用现有 Comment 模型，成功后前端重新查询姓名。
- 价格输入先拆分元与小数，再转换整数分，避免浮点乘法误差。
- 列表分页、分类/成色筛选和排序目前在浏览器完成；数据库分页尚待后端设计。
- 不包含预约、物流、退款、在线注册、图片上传和管理员后台。图片为根据书名生成的文字示意。

## 文件组织与维护

- 各 HTML 文件：独立页面外壳，以 `body[data-page]` 选择页面。
- `app.js`：带中文注释的页面渲染、表单校验、登录守卫和交互。
- `api.js`：统一请求、超时、响应解析、错误状态及凭据管理。
- `style.css`：展开后的现有样式，以及登录、订单和响应式补充样式。
- `server.mjs`：固定白名单的静态服务与 API 代理。

页面没有内置业务样例数据。后续增加页面时，需要同时增加 HTML 文件、app.js 页面函数和 server.mjs 静态文件白名单。

## 本次验证与边界

使用真实浏览器加载真实前端，浏览器内隔离的 API 响应用于检查：登录失败/成功、密码显隐、分页和课程搜索、留言转义/发布/删除、编辑金额与清空描述、下架和历史留言、订单创建与支付返回、取消与确认收货、未实现接口的文本404提示、保存失败保留输入、手机布局以及退出登录。

上述浏览器验证用于检查前端行为。交付阶段另已通过连接真实 SQL Server 的 API 集成测试，覆盖身份、归属、教材、留言、订单与并发下单；相关测试仅创建并清理临时测试记录。付款成功由用户实际沙箱操作确认，自动化未代为付款。详见 ../docs/验收记录.md。

## 支付接口完成后的验证

付款返回页主动调用 sync，刷新按钮也主动查单；订单为 paid 时可确认收货，取消时遇到平台状态不明会保留占用并提示重试。真实支付宝沙箱只读查单已通过；用户确认沙箱付款成功且项目状态正确。自动化没有单独证明该次付款由异步通知还是主动查单完成入账。
