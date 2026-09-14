package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/internal/auth"
)

// Login 管理员登录,签发 JWT。
func (h *Handler) Login(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	cfg := h.svc.Cfg
	if body.Username != cfg.AdminUser || body.Password != cfg.AdminPass {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账号或密码错误"})
		return
	}
	tok, err := auth.IssueToken(cfg.JWTSecret, cfg.AdminUser, cfg.JWTTTL())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "issue token: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": tok, "username": cfg.AdminUser})
}

// Me 返回当前管理员身份。
func (h *Handler) Me(c *gin.Context) {
	user := c.GetString("admin_user")
	if user == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"username": user})
}

// Logout 客户端清 token;服务端无状态,直接成功。
func (h *Handler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
