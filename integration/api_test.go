package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"campus-books/models"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 显式启用才对运行中的真实API验收。创建独立测试账号，最后只清理这些账号的数据。
// 付款链接只生成、不打开；不会向支付宝创建真实交易或进行付款。
func TestLiveAPI(t *testing.T) {
	if os.Getenv("CAMPUS_API_TEST") != "1" {
		t.Skip("设置CAMPUS_API_TEST=1显式运行真实API验收")
	}
	values, err := godotenv.Read("../.env")
	if err != nil {
		t.Fatal("无法读取测试环境配置")
	}
	db, err := gorm.Open(sqlserver.Open(values["DB_DSN"]), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("无法连接验收数据库")
	}
	pool, _ := db.DB()
	defer pool.Close()
	prefix := fmt.Sprintf("验收%d", time.Now().UnixNano())
	password := fmt.Sprintf("qa-%d", time.Now().UnixNano())
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	users := []models.User{{Name: prefix + "甲", PasswordHash: string(hash)}, {Name: prefix + "乙", PasswordHash: string(hash)}, {Name: prefix + "丙", PasswordHash: string(hash)}}
	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatal("创建临时账号失败")
		}
	}
	defer func() {
		ids := []int64{users[0].ID, users[1].ID, users[2].ID}
		cleanupErr := db.Transaction(func(tx *gorm.DB) error {
			var bookIDs, orderIDs []int64
			if err := tx.Model(&models.Book{}).Where("seller_id IN ?", ids).Pluck("id", &bookIDs).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.Order{}).Where("buyer_id IN ? OR seller_id IN ?", ids, ids).Pluck("id", &orderIDs).Error; err != nil {
				return err
			}
			if len(orderIDs) > 0 {
				if err := tx.Where("order_id IN ?", orderIDs).Delete(&models.Payment{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("buyer_id IN ? OR seller_id IN ?", ids, ids).Delete(&models.Order{}).Error; err != nil {
				return err
			}
			if len(bookIDs) > 0 {
				if err := tx.Where("book_id IN ?", bookIDs).Delete(&models.Comment{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("user_id IN ?", ids).Delete(&models.Comment{}).Error; err != nil {
				return err
			}
			if err := tx.Where("seller_id IN ?", ids).Delete(&models.Book{}).Error; err != nil {
				return err
			}
			return tx.Where("id IN ? AND name LIKE ?", ids, prefix+"%").Delete(&models.User{}).Error
		})
		if cleanupErr != nil {
			t.Error("临时验收数据清理失败，请检查验收前缀", prefix)
		}
	}()
	base := os.Getenv("CAMPUS_API_BASE")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	client := &http.Client{Timeout: 35 * time.Second}
	call := func(method, path, token string, body interface{}, want int) map[string]interface{} {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req, err := http.NewRequest(method, base+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		r, err := client.Do(req)
		if err != nil {
			t.Fatal("API请求失败", method, path)
		}
		defer r.Body.Close()
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if r.StatusCode != want {
			t.Fatalf("%s %s 状态%d，预期%d；message=%v", method, path, r.StatusCode, want, payload["message"])
		}
		return payload
	}
	tokenFor := func(name string) string {
		p := call("POST", "/api/login", "", map[string]string{"name": name, "password": password}, 200)
		return p["data"].(map[string]interface{})["token"].(string)
	}
	seller, buyer, outsider := tokenFor(users[0].Name), tokenFor(users[1].Name), tokenFor(users[2].Name)
	call("POST", "/api/login", "", map[string]string{"name": users[0].Name, "password": "wrong"}, 401)
	payload := map[string]interface{}{"title": prefix + "Go教材", "course": "Go程序设计", "category": "计算机", "condition": "九成新", "price": 1999, "description": "验收创建，自动清理", "contact": "验收联系方式"}
	created := call("POST", "/api/books", seller, payload, 201)["data"].(map[string]interface{})
	bookID := int64(created["id"].(float64))
	bookPath := fmt.Sprintf("/api/books/%d", bookID)
	call("POST", "/api/books", "", payload, 401)
	detail := call("GET", bookPath, "", nil, 200)["data"].(map[string]interface{})
	if detail["seller_name"] != users[0].Name || detail["category"] != "计算机" {
		t.Fatal("姓名或分类未持久化")
	}
	call("GET", "/api/books?keyword="+url.QueryEscape(prefix), "", nil, 200)
	call("PUT", bookPath, outsider, payload, 403)
	payload["price"] = 2099
	payload["description"] = ""
	call("PUT", bookPath, seller, payload, 200)
	detail = call("GET", bookPath, "", nil, 200)["data"].(map[string]interface{})
	if detail["price"] != float64(2099) || detail["description"] != "" {
		t.Fatal("价格或清空描述失败")
	}
	comment := call("POST", bookPath+"/comments", buyer, map[string]string{"content": "这本教材是什么版次？"}, 201)["data"].(map[string]interface{})
	call("POST", bookPath+"/comments", buyer, map[string]string{"content": "   "}, 400)
	commentPath := fmt.Sprintf("/api/comments/%.0f", comment["id"].(float64))
	call("DELETE", commentPath, outsider, nil, 403)
	call("DELETE", commentPath, buyer, nil, 200)
	call("POST", bookPath+"/comments", buyer, map[string]string{"content": "历史留言保留测试"}, 201)
	makeOrder := func() int64 {
		return int64(call("POST", "/api/orders", buyer, map[string]int64{"book_id": bookID}, 201)["data"].(map[string]interface{})["id"].(float64))
	}
	id := makeOrder()
	path := fmt.Sprintf("/api/orders/%d", id)
	call("POST", "/api/orders", outsider, map[string]int64{"book_id": bookID}, 409)
	call("GET", path, outsider, nil, 404)
	call("GET", path, seller, nil, 200)
	call("GET", "/api/orders?role=buy", buyer, nil, 200)
	call("GET", "/api/orders?role=sell", seller, nil, 200)
	call("PATCH", bookPath+"/status", seller, map[string]string{"status": "off_shelf"}, 409)
	call("POST", path+"/cancel", seller, nil, 404)
	call("POST", path+"/cancel", buyer, nil, 200)
	second := makeOrder()
	call("POST", path+"/cancel", buyer, nil, 200)
	var reserved models.Book
	db.First(&reserved, bookID)
	if reserved.Status != "reserved" {
		t.Fatal("旧取消错误释放了新占用")
	}
	call("POST", fmt.Sprintf("/api/orders/%d/cancel", second), buyer, nil, 200)
	// 两个真实HTTP并发下单请求，必须恰好一个201、一个409。
	codes := make(chan int, 2)
	for _, token := range []string{buyer, outsider} {
		go func(token string) {
			raw, _ := json.Marshal(map[string]int64{"book_id": bookID})
			req, _ := http.NewRequest("POST", base+"/api/orders", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			r, e := client.Do(req)
			if e != nil {
				codes <- 0
				return
			}
			defer r.Body.Close()
			codes <- r.StatusCode
		}(token)
	}
	a, b := <-codes, <-codes
	if !((a == 201 && b == 409) || (a == 409 && b == 201)) {
		t.Fatalf("并发下单状态：%d %d", a, b)
	}
	var pending models.Order
	db.Where("book_id = ? AND status = ?", bookID, "pending").First(&pending)
	activeToken := buyer
	if pending.BuyerID == users[2].ID {
		activeToken = outsider
	}
	payPath := fmt.Sprintf("/api/orders/%d", pending.ID)
	call("POST", payPath+"/complete", activeToken, nil, 409)
	// 只生成签名链接，不打开链接，不调用平台付款。
	payment := call("POST", payPath+"/pay", activeToken, nil, 200)["data"].(map[string]interface{})
	address, err := url.Parse(payment["payment_url"].(string))
	if err != nil || address.Scheme != "https" || !strings.Contains(address.Host, "sandbox") {
		t.Fatal("付款链接不是沙箱HTTPS")
	}
	// 删除临时账号后，原JWT不能继续写入，这是本次收尾补充的校验。
	if err := db.Delete(&users[2]).Error; err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/books", outsider, payload, 401)
	t.Log("真实登录/分类/姓名/编辑/留言/权限/订单/旧取消/并发下单/付款链接/已删账号校验通过；临时数据随后清理")
}
