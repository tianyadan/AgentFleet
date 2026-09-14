package auth

import (
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// GinIPAllowlist 局域网 IP 白名单中间件。空列表=放行全部。
// 直接检查 c.ClientIP() 并通过 c.AbortWithStatus 拒绝,不包装 ResponseWriter,
// 以免破坏 SSE 的 http.Flusher 能力。
func GinIPAllowlist(allowed []string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, ip := range allowed {
		set[strings.TrimSpace(ip)] = struct{}{}
	}
	return func(c *gin.Context) {
		if len(set) == 0 {
			c.Next()
			return
		}
		host := c.ClientIP()
		if _, ok := set[host]; !ok {
			c.AbortWithStatusJSON(403, gin.H{"error": "forbidden: source IP not allowed"})
			return
		}
		c.Next()
	}
}

// Authorized 判定 candidate 是否落在 authorizedRoots 任一授权工作区内(需绝对路径)。
// 返回命中的授权根路径;未命中 ok=false。
func Authorized(candidate string, authorizedRoots []string) (matched string, ok bool) {
	if candidate == "" {
		return "", false
	}
	abs := filepath.Clean(candidate)
	// 需为绝对路径
	if !filepath.IsAbs(abs) {
		return "", false
	}
	for _, root := range authorizedRoots {
		r := filepath.Clean(root)
		if abs == r {
			return r, true
		}
		rel, err := filepath.Rel(r, abs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return r, true
		}
	}
	return "", false
}
