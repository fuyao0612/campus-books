package main

import (
	"log"
	"net/http"
	"os"

	"campus-books/models"

	"campus-books/database"
	"campus-books/handlers"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件")
	}

	// 从环境变量读取数据库连接字符串
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("请先设置 DB_DSN 环境变量")
	}

	//从环境变量中读取JWT签名密钥
	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		log.Fatal("jwt中至少使用32字节的随机密钥")
	}

	//调用database包的Connect函数连接数据库
	db, err := database.Connect(dsn)
	if err != nil {
		log.Fatalf("数据库连接失败：%v", err)
	}

	//取得数据库连接池对象
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("获取数据库连接池失败：%v", err)
	}
	defer sqlDB.Close()

	log.Println("数据库连接成功")

	if err := db.AutoMigrate(
		&models.Book{},
		&models.User{},
		&models.Comment{},
	); err != nil {
		log.Fatalf("教材表初始化失败：%v", err)
	}
	log.Println("教材表初始化成功")

	// 创建 Gin 路由对象，默认启用请求日志和 panic 恢复中间件。
	router := gin.Default()

	// 创建认证处理器：数据库用于查询用户，密钥用于签发和验证 JWT。
	authHandler := &handlers.AuthHandler{
		DB:        db,
		JWTSecret: []byte(jwtSecret),
	}

	// 创建教材处理器，将已连接的数据库对象交给教材接口使用。
	bookHandler := &handlers.BookHandler{DB: db}

	// 创建留言处理器，将已连接的数据库对象交给留言接口使用。
	commentHandler := &handlers.CommentHandler{DB: db}

	// 服务检查：无需登录，只返回服务运行提示，不实时检查数据库连接。
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "后端服务运行正常",
		})
	})

	// 用户认证：登录无需令牌，成功后返回 JWT。
	router.POST("/api/login", authHandler.Login)

	// 当前用户：先验证 JWT，再根据令牌中的用户编号查询用户信息。
	router.GET("/api/me", authHandler.RequireAuth(), authHandler.Me)

	// 教材查询：无需登录；列表支持关键词搜索，详情通过路径参数 id 指定教材。
	router.GET("/api/books", bookHandler.List)
	router.GET("/api/books/:id", bookHandler.Detail)

	// 发布教材：先验证 JWT 并保存 user_id，再由 Create 校验内容并写入数据库。
	router.POST(
		"/api/books",
		authHandler.RequireAuth(),
		bookHandler.Create,
	)

	// 留言查询：无需登录，按路径参数 id 查询指定教材的留言。
	router.GET("/api/books/:id/comments", commentHandler.List)

	// 发布留言：先验证 JWT，再使用当前用户编号为指定教材创建留言。
	router.POST(
		"/api/books/:id/comments",
		authHandler.RequireAuth(),
		commentHandler.Create,
	)

	// 完成处理器初始化和路由登记后，监听 8080 端口并开始接收 HTTP 请求。
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("服务启动失败：%v", err)
	}
}
