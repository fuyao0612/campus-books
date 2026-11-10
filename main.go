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

	if err := db.AutoMigrate(&models.Book{}); err != nil {
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

	//注册教材查询接口
	router.GET("/api/books", bookHandler.List)

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("服务启动失败：%v", err)
	}
}
