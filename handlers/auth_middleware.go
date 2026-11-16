package handlers

import (
	"campus-books/models"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// 验证JWT，并把用户编号保存到本次请求的上下文中
func (h *AuthHandler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		//读取请求头中的Authorization
		authHeader := c.GetHeader("Authorization")

		//按空白字符拆分，预期得到Bearer和令牌
		parts := strings.Fields(authHeader)

		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "请先登录并提供有效令牌",
			})
			return
		}

		// 准备接收令牌中的用户信息。
		claims := &AuthClaims{}

		// 解析令牌，验证签名、签发者和有效期。
		token, err := jwt.ParseWithClaims(
			parts[1],
			claims,
			func(token *jwt.Token) (interface{}, error) {
				return h.JWTSecret, nil
			},
			jwt.WithValidMethods([]string{"HS256"}),
			jwt.WithIssuer("campus-books"),
			jwt.WithExpirationRequired(),
		)

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "登录已失效，请重新登录",
			})
			return
		}

		// 有效用户编号必须大于零。
		if claims.UserID <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "令牌中的用户信息无效",
			})
			return
		}
		// 令牌未过期不代表账号仍存在，避免已删除账号继续写入数据。
		var count int64
		if err := h.DB.WithContext(c.Request.Context()).Model(&models.User{}).Where("id = ?", claims.UserID).Count(&count).Error; err != nil {
			log.Printf("检查登录账号失败：%v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "暂时无法验证登录，请稍后重试"})
			return
		}
		if count != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "账号不存在，请重新登录"})
			return
		}

		// 保存当前用户编号，供后面的处理函数使用。
		c.Set("user_id", claims.UserID)

		// 继续执行后面的处理函数。
		c.Next()
	}
}
