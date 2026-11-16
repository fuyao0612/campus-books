package handlers

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequireAuthDeletedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, exists := range []bool{true, false} {
		t.Run(map[bool]string{true: "账号有效", false: "已删除账号拒绝"}[exists], func(t *testing.T) {
			conn, m, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			db, err := gorm.Open(sqlserver.New(sqlserver.Config{Conn: conn}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			secret := []byte("test-only-secret-at-least-32-bytes-long")
			claims := AuthClaims{UserID: 3, RegisteredClaims: jwt.RegisteredClaims{Issuer: "campus-books", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
			token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
			count := 0
			if exists {
				count = 1
			}
			m.ExpectQuery(`SELECT count\(\*\).*users.*`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
			h := &AuthHandler{DB: db, JWTSecret: secret}
			r := gin.New()
			r.GET("/protected", h.RequireAuth(), func(c *gin.Context) { c.Status(204) })
			req := httptest.NewRequest("GET", "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			want := 401
			if exists {
				want = 204
			}
			if w.Code != want {
				t.Fatal(w.Code)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
