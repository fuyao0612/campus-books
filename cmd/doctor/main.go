// doctor只检查运行环境，不输出DSN、密码、账号姓名或密钥，不迁移数据库。
package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/joho/godotenv"
	_ "github.com/microsoft/go-mssqldb"
	"os"
	"time"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		fmt.Println("数据库配置缺失")
		os.Exit(1)
	}
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		fmt.Println("数据库连接配置无效")
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var version, edition string
	if err = db.QueryRowContext(ctx, "SELECT CAST(SERVERPROPERTY('ProductVersion') AS nvarchar(128)), CAST(SERVERPROPERTY('Edition') AS nvarchar(128))").Scan(&version, &edition); err != nil {
		fmt.Println("数据库检查失败，请检查服务、TCP端口和账号权限")
		os.Exit(1)
	}
	fmt.Printf("数据库连接成功；SQL Server版本：%s；版本类型：%s\n", version, edition)
	var canCreate int
	if err = db.QueryRowContext(ctx, "SELECT COALESCE(HAS_PERMS_BY_NAME(NULL,NULL,'CREATE ANY DATABASE'),0)").Scan(&canCreate); err == nil {
		fmt.Printf("隔离测试建库权限：%t\n", canCreate == 1)
	}
	for _, table := range []string{"users", "books", "comments", "orders", "payments"} {
		var exists int
		if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA='dbo' AND TABLE_NAME=@p1", table).Scan(&exists); err != nil {
			fmt.Println("表检查失败")
			os.Exit(1)
		}
		fmt.Printf("表 %s 存在：%t\n", table, exists == 1)
	}
}
