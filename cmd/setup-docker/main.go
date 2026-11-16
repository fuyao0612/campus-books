// setup-docker从现有本机配置生成容器数据库连接，不输出账号、密码或密钥。
package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	values, err := godotenv.Read(".env")
	if err != nil {
		fmt.Fprintln(os.Stderr, "请先复制.env.example为.env并填写配置")
		os.Exit(1)
	}
	if values["DOCKER_DB_DSN"] != "" {
		fmt.Println("DOCKER_DB_DSN已配置，保留原值")
		return
	}
	u, err := url.Parse(values["DB_DSN"])
	if err != nil || u.Scheme != "sqlserver" || u.Hostname() == "" {
		fmt.Fprintln(os.Stderr, "DB_DSN必须是有效的sqlserver URL")
		os.Exit(1)
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		host = "host.docker.internal"
	}
	if u.Port() != "" {
		u.Host = net.JoinHostPort(host, u.Port())
	} else {
		u.Host = host
	}
	if u.User != nil {
		password, ok := u.User.Password()
		if ok {
			u.User = url.UserPassword(u.User.Username(), password)
		} else {
			u.User = url.User(u.User.Username())
		}
	}
	dsn := u.String()
	if strings.ContainsAny(dsn, "'\r\n") {
		fmt.Fprintln(os.Stderr, "连接字符串包含不支持的字符，请手动配置DOCKER_DB_DSN")
		os.Exit(1)
	}
	f, err := os.OpenFile(".env", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法更新本机配置")
		os.Exit(1)
	}
	defer f.Close()
	if _, err = fmt.Fprintf(f, "\n# Docker容器访问Windows宿主机数据库。\nDOCKER_DB_DSN='%s'\n", dsn); err != nil {
		fmt.Fprintln(os.Stderr, "写入配置失败")
		os.Exit(1)
	}
	fmt.Println("已生成DOCKER_DB_DSN，原DB_DSN和其他配置保持不变")
}
