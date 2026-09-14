package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// adminClaims 管理员 JWT 载荷。
type adminClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// IssueToken 签发管理员 JWT。
func IssueToken(secret, username string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := adminClaims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

// ParseToken 校验并解析管理员 JWT,返回用户名。
func ParseToken(secret, tokenStr string) (string, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &adminClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := t.Claims.(*adminClaims)
	if !ok || !t.Valid {
		return "", jwt.ErrTokenInvalidClaims
	}
	if claims.Username == "" {
		claims.Username = claims.Subject
	}
	return claims.Username, nil
}

// RequireAdminJWT Gin 中间件:校验 Bearer JWT,成功后写入 admin_user。
func RequireAdminJWT(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		user, err := ParseToken(secret, strings.TrimSpace(h[7:]))
		if err != nil || user == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("admin_user", user)
		c.Next()
	}
}
