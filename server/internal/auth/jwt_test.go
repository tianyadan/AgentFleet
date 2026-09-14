package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestIssueAndParseToken(t *testing.T) {
	secret := "test-secret"
	tok, err := IssueToken(secret, "tianhaowen", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	user, err := ParseToken(secret, tok)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if user != "tianhaowen" {
		t.Fatalf("username = %q, want tianhaowen", user)
	}
}

func TestParseTokenRejectsBadSignature(t *testing.T) {
	tok, err := IssueToken("secret-a", "tianhaowen", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken("secret-b", tok); err == nil {
		t.Fatal("expected error for bad signature")
	}
}

func TestParseTokenRejectsExpired(t *testing.T) {
	tok, err := IssueToken("secret", "tianhaowen", -time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken("secret", tok); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestRequireAdminJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "mw-secret"
	tok, err := IssueToken(secret, "tianhaowen", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.GET("/x", RequireAdminJWT(secret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user": c.GetString("admin_user")})
	})

	// 无 token → 401
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want 401", w.Code)
	}

	// 有效 token → 200
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("valid token: status %d, want 200", w.Code)
	}
}
