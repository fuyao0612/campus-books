package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"campus-books/models"

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

	// 查询所有在售教材。
	router.GET("/api/books", func(c *gin.Context) {
		// 获取查询参数 keyword
		keyword := strings.TrimSpace(c.Query("keyword"))

		// 创建切片，用来接收查询结果。
		books := make([]models.Book, 0)

		//基础条件，只查询在售教材
		query := db.Where("status=?", "on_sale")

		//有关键词时再增加书名搜索条件
		if keyword != "" {
			query = query.Where("title LIKE ?", "%"+keyword+"%")
		}
		// 执行查询
		result := query.Order("id DESC").Find(&books)

		if result.Error != nil {
			log.Printf("查询教材失败：%v", result.Error)

			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "查询教材失败，请稍后重试",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "查询成功",
			"data":    books,
		})
	})

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("服务启动失败：%v", err)
	}
}
