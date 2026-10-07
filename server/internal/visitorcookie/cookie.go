package visitorcookie

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

const CookieName = "avatar_visitor"
const MaxAge = 7 * 24 * time.Hour

func hmacHex(secret, publicID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(publicID))
	return hex.EncodeToString(mac.Sum(nil))
}

// Sign 生成 Cookie 值：base64url(publicID).hex(hmac)。
func Sign(secret, publicID string) string {
	publicID = strings.TrimSpace(publicID)
	return base64.RawURLEncoding.EncodeToString([]byte(publicID)) + "." + hmacHex(secret, publicID)
}

// Parse 校验并取出 publicID。
func Parse(secret, raw string) (publicID string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || secret == "" {
		return "", false
	}
	i := strings.LastIndex(raw, ".")
	if i <= 0 || i >= len(raw)-1 {
		return "", false
	}
	pidBytes, err := base64.RawURLEncoding.DecodeString(raw[:i])
	if err != nil || len(pidBytes) == 0 {
		return "", false
	}
	publicID = string(pidBytes)
	if !hmac.Equal([]byte(hmacHex(secret, publicID)), []byte(raw[i+1:])) {
		return "", false
	}
	return publicID, true
}

// Set 写入 HttpOnly Cookie（7 天）。
func Set(w http.ResponseWriter, secret, publicID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    Sign(secret, publicID),
		Path:     "/",
		MaxAge:   int(MaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// Clear 清除访客 Cookie。
func Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// FromRequest 从请求读取并验签。
func FromRequest(r *http.Request, secret string) (publicID string, ok bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c == nil {
		return "", false
	}
	return Parse(secret, c.Value)
}
