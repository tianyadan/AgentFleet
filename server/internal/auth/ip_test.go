package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIPAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinIPAllowlist([]string{"10.0.0.5", "127.0.0.1"}))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 允许
	req := httptest.NewRequest("GET", "/ping", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("allowed IP got %d", w.Code)
	}

	// 拒绝
	req2 := httptest.NewRequest("GET", "/ping", nil)
	req2.RemoteAddr = "192.168.1.99:1234"
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 403 {
		t.Fatalf("blocked IP got %d, want 403", w2.Code)
	}
}
