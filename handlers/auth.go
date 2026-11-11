package handlers

import (
	"campus-books/models"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 保存登录接口需要使用的资源
type AuthHandler struct {
	DB        *gorm.DB
	JWTSecret []byte
}

// 前端发来的登录数据
type LoginRequest struct {
	Name     string `json:"name" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// JWT中保存用户信息和有效期
type AuthClaims struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

// 验证用户并签发JWT
func (h *AuthHandler) Login(c *gin.Context) {
	var request LoginRequest

	//把请求json转成GO结构体，并检查必填字段
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "请填写姓名和学号",
		})
	}

	// 去掉姓名两端空白；密码不做 Trim，避免改变用户输入。
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "姓名不能为空",
		})
		return
	}

	// 查找同名用户。姓名可能重名，所以使用切片接收查询结果。
	var users []models.User
	result := h.DB.Where("name = ?", request.Name).Find(&users)
	if result.Error != nil {
		log.Printf("查询登录用户失败：%v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "登录失败，请稍后重试",
		})
		return
	}

	// 逐个比较同名用户的密码哈希。
	var matchedUser *models.User
	for i := range users {
		err := bcrypt.CompareHashAndPassword(
			[]byte(users[i].PasswordHash),
			[]byte(request.Password),
		)
		if err == nil {
			matchedUser = &users[i]
			break
		}
	}

	// 姓名不存在或密码不匹配时，返回相同提示。
	if matchedUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "姓名或学号错误",
		})
		return
	}

	// 创建有效期为两小时的 JWT 声明。
	now := time.Now()
	claims := AuthClaims{
		UserID: matchedUser.ID,
		Name:   matchedUser.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "campus-books",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(2 * time.Hour)),
		},
	}

	// 指定 HS256 算法，并用后端密钥给令牌签名。
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.JWTSecret)
	if err != nil {
		log.Printf("生成登录令牌失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "登录失败，请稍后重试",
		})
		return
	}

	// 只返回必要的用户信息，不返回密码哈希。
	c.JSON(http.StatusOK, gin.H{
		"message": "登录成功",
		"data": gin.H{
			"token":      tokenString,
			"token_type": "Bearer",
			"expires_in": 7200,
			"user": gin.H{
				"id":   matchedUser.ID,
				"name": matchedUser.Name,
			},
		},
	})
}
