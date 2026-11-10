package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
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

	// 创建 GORM 数据库对象
	db, err := gorm.Open(sqlserver.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("数据库初始化失败：%v", err)
	}

	// 获取底层数据库连接池
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("获取数据库连接池失败：%v", err)
	}
	defer sqlDB.Close()

	// 实际检查数据库连接
	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("数据库连接检查失败：%v", err)
	}

	log.Println("数据库连接成功")

	// 启动 HTTP 服务
	router := gin.Default()

	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "后端服务运行正常",
		})
	})

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("服务启动失败：%v", err)
	}
}
