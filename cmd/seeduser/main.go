package main

import (
	"campus-books/database"
	"campus-books/models"
	"log"
	"os"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	//把.env中的配置读入环境变量
	if err := godotenv.Load(); err != nil {
		log.Fatal("读取.env失败")
	}

	dsn := os.Getenv("DB_DSN")
	name := os.Getenv("SEED_USER_NAME")
	password := os.Getenv("SEED_USER_PASSWORD")

	if dsn == "" || name == "" || password == "" {
		log.Fatal("请检查.env中的数据库和用户配置")
	}

	//拿到Gorm对象db
	db, err := database.Connect(dsn)
	if err != nil {
		log.Fatalf("连接数据库失败:%v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("获取数据库连接池失败:%v", err)
	}
	defer sqlDB.Close()

	var sameNameUsers []models.User
	if err := db.Where("name=?", name).Find(&sameNameUsers).Error; err != nil {
		log.Fatalf("查询用户失败:%v", err)
	}

	for _, user := range sameNameUsers {
		err := bcrypt.CompareHashAndPassword(
			[]byte(user.PasswordHash),
			[]byte(password),
		)
		if err == nil {
			log.Println("该账号已存在")
			return
		}
	}

	//把学号转换为哈希
	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)

	if err != nil {
		log.Fatalf("生成密码哈希失败：%v", err)
	}

	user := models.User{
		Name:         name,
		PasswordHash: string(passwordHash),
	}

	//把用户信息写入数据库
	if err := db.Create(&user).Error; err != nil {
		log.Fatalf("创建用户失败：%v", err)
	}

	log.Printf("用户创建成功，编号：%d", user.ID)
}
