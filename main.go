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

	if err := db.AutoMigrate(&models.Book{}, &models.User{}); err != nil {
		log.Fatalf("教材表初始化失败：%v", err)
	}
	log.Println("教材表初始化成功")

	// 启动 HTTP 服务
	router := gin.Default()

	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "后端服务运行正常",
		})
	})

	//创建教材处理器，把已经连接好的数据库对象接入
	bookHandler := &handlers.BookHandler{DB: db}

	//创建登录处理器，提供数据库和JWT签名密钥
	authHandler := &handlers.AuthHandler{
		DB:        db,
		JWTSecret: []byte(jwtSecret),
	}

	//注册教材查询接口
	router.GET("/api/books", bookHandler.List)
	// 发布教材前，先验证登录身份。
	router.POST("/api/books", authHandler.RequireAuth(), bookHandler.Create)
	router.GET("/api/books/:id", bookHandler.Detail)

	//注册登录接口
	router.POST("/api/login", authHandler.Login)

	//先验证登录状态，再查询当前用户
	router.GET("/api/me", authHandler.RequireAuth(), authHandler.Me)

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("服务启动失败：%v", err)
	}
}
