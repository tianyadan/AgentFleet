package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/config"
	"colleague-avatar/server/internal/auth"
	"colleague-avatar/server/internal/permission"
	"colleague-avatar/server/internal/service"
)

// TestPermissionPendingAdminSeesAdminOwner 管理 JWT 应能看到 user_ip=admin 的挂起授权。
func TestPermissionPendingAdminSeesAdminOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := permission.NewHub(60)
	hub.Register("Bash", map[string]interface{}{"command": "ls"}, "ls", "", 42, "admin")

	h := &Handler{svc: &service.Service{
		Cfg:   config.Config{JWTSecret: "test-jwt-secret", AdminUser: "tianhaowen", AdminPass: "12345678"},
		Perms: hub,
	}}
	r := gin.New()
	h.Register(r)

	// 无 JWT：浏览器 IP 与 admin 不符，应被过滤
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/permissions/pending", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	r.ServeHTTP(w, req)
	var bare map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &bare)
	items, _ := bare["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("without JWT want 0 items, got %d body=%s", len(items), w.Body.String())
	}

	tok, err := auth.IssueToken("test-jwt-secret", "tianhaowen", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/permissions/pending", nil)
	req2.RemoteAddr = "192.0.2.10:12345"
	req2.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w2, req2)
	var withAdmin map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &withAdmin)
	items2, _ := withAdmin["items"].([]any)
	if len(items2) != 1 {
		t.Fatalf("with admin JWT want 1 item, got %d body=%s", len(items2), w2.Body.String())
	}
}
