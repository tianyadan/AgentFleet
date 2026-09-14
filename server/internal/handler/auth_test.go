package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/config"
	"colleague-avatar/server/internal/service"
)

func testAuthHandler() *Handler {
	cfg := config.Config{
		AdminUser:   "tianhaowen",
		AdminPass:   "12345678",
		JWTSecret:   "test-jwt-secret",
		JWTTTLHours: 24,
	}
	return New(&service.Service{Cfg: cfg})
}

func TestLoginWrongPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testAuthHandler()
	r := gin.New()
	h.Register(r)

	body, _ := json.Marshal(map[string]string{"username": "tianhaowen", "password": "wrong"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401; body=%s", w.Code, w.Body.String())
	}
}

func TestLoginAndMe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testAuthHandler()
	r := gin.New()
	h.Register(r)

	body, _ := json.Marshal(map[string]string{"username": "tianhaowen", "password": "12345678"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status %d, body=%s", w.Code, w.Body.String())
	}
	var loginResp struct {
		Token    string `json:"token"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &loginResp); err != nil {
		t.Fatal(err)
	}
	if loginResp.Token == "" || loginResp.Username != "tianhaowen" {
		t.Fatalf("bad login resp: %+v", loginResp)
	}

	w = httptest.NewRecorder()
	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	r.ServeHTTP(w, meReq)
	if w.Code != http.StatusOK {
		t.Fatalf("me status %d, body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("me without token: %d", w.Code)
	}
}

func TestProtectedConversationsRequireJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testAuthHandler()
	r := gin.New()
	h.Register(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/conversations", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("conversations without JWT: %d, want 401", w.Code)
	}
}

func TestLogoutOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testAuthHandler()
	r := gin.New()
	h.Register(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("logout status %d", w.Code)
	}
}
