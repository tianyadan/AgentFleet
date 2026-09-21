package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminAgentsCreateRequiresJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testAuthHandler()
	r := gin.New()
	h.Register(r)

	body, _ := json.Marshal(map[string]string{"name": "dev", "engine": "claude"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", w.Code)
	}
}

func TestAdminAgentsCreateRejectsBadEngine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testAuthHandler()
	r := gin.New()
	h.Register(r)

	// login
	loginBody, _ := json.Marshal(map[string]string{"username": "tianhaowen", "password": "12345678"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var lr struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &lr)
	if lr.Token == "" {
		t.Fatal("no token")
	}

	body, _ := json.Marshal(map[string]string{"name": "x", "engine": "cursor"})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+lr.Token)
	r.ServeHTTP(w, req)
	// Store is nil → may 500 after validation; bad engine should 400 before DB
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d body=%s, want 400", w.Code, w.Body.String())
	}
}

func TestAdminAgentsCopyRequiresJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testAuthHandler()
	r := gin.New()
	h.Register(r)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/agents/1/copy", bytes.NewReader([]byte(`{"name":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", w.Code)
	}
}

func TestAdminAgentsCopyRejectsEmptyName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testAuthHandler()
	r := gin.New()
	h.Register(r)
	loginBody, _ := json.Marshal(map[string]string{"username": "tianhaowen", "password": "12345678"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var lr struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &lr)
	if lr.Token == "" {
		t.Fatal("no token")
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/agents/1/copy", bytes.NewReader([]byte(`{"name":"  "}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+lr.Token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d body=%s, want 400", w.Code, w.Body.String())
	}
}
