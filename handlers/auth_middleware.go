package handlers

import (
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

		// 保存当前用户编号，供后面的处理函数使用。
		c.Set("user_id", claims.UserID)

		// 继续执行后面的处理函数。
		c.Next()
	}
}
