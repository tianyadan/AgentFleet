# Admin Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (inline). Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add admin JWT login and split public chat vs admin shell (history/stats behind auth).

**Architecture:** Single admin credentials from env; HS256 JWT in localStorage; `RequireAdminJWT` on management routes; React view switch `chat | admin` with left nav.

**Tech Stack:** Go + Gin + golang-jwt/jwt/v5; React JSX (no frontend unit tests).

## Global Constraints

- Admin defaults: user `tianhaowen`, pass `12345678`
- JWT secret default: `colleague-avatar-jwt-dev-secret-change-me`
- JWT TTL: 168 hours
- localStorage key: `avatar_admin_token`
- Frontend: no unit tests
- Do not commit unless user asks

## File Structure

- Create: `server/internal/auth/jwt.go` — IssueToken / ParseToken / RequireAdminJWT
- Create: `server/internal/auth/jwt_test.go`
- Create: `server/internal/handler/auth.go` — Login / Me / Logout
- Create: `server/internal/handler/auth_test.go`
- Modify: `server/config/config.go` — AdminUser, AdminPass, JWTSecret, JWTTTLHours
- Modify: `server/internal/handler/handler.go` — route groups
- Modify: `server/go.mod` / `go.sum` — jwt dependency
- Modify: `web/src/App.jsx` — login, admin shell, auth fetch
- Modify: `web/src/App.css` — login modal + admin layout

---

### Task 1: JWT helpers + config

**Files:**
- Create: `server/internal/auth/jwt.go`
- Create: `server/internal/auth/jwt_test.go`
- Modify: `server/config/config.go`

**Interfaces:**
- Produces: `IssueToken(secret, username string, ttl time.Duration) (string, error)`
- Produces: `ParseToken(secret, token string) (username string, err error)`
- Produces: `RequireAdminJWT(secret string) gin.HandlerFunc`
- Produces: Config fields `AdminUser`, `AdminPass`, `JWTSecret`, `JWTTTLHours` + `JWTTTL() time.Duration`

- [ ] **Step 1:** Write `jwt_test.go` (issue/parse roundtrip, bad sig, expired)
- [ ] **Step 2:** Run tests — expect FAIL
- [ ] **Step 3:** Implement `jwt.go` + config fields; `go get github.com/golang-jwt/jwt/v5`
- [ ] **Step 4:** Run tests — expect PASS

---

### Task 2: Auth handlers + protect routes

**Files:**
- Create: `server/internal/handler/auth.go`
- Create: `server/internal/handler/auth_test.go`
- Modify: `server/internal/handler/handler.go` `Register`

**Interfaces:**
- Consumes: config admin creds + IssueToken/ParseToken/RequireAdminJWT
- Produces: `POST /api/auth/login`, `GET /api/auth/me`, `POST /api/auth/logout`
- Protect: conversations GET list/messages, stats*, commands, agents*, test-servers*, db*

- [ ] **Step 1:** Write handler tests with gin test context / httptest
- [ ] **Step 2:** Run — expect FAIL
- [ ] **Step 3:** Implement handlers + regroup routes
- [ ] **Step 4:** Run — expect PASS

---

### Task 3: Frontend login + admin shell

**Files:**
- Modify: `web/src/App.jsx`, `web/src/App.css`

**Behavior:**
- Top-right Login / 管理台; modal; JWT localStorage
- view chat|admin; admin left menu; history/stats migrated; placeholders
- authHeaders() on protected fetches; soft-fail loadSessions when no token
- 401 on protected → clear auth, back to chat
- History item → open conversation + return to chat (token kept)
- No frontend unit tests

- [ ] **Step 1:** Implement UI + auth flow
- [ ] **Step 2:** Manual smoke / backend tests still pass

---

### Task 4: Verify

- [ ] `cd server && go test ./internal/auth/ ./internal/handler/ -count=1`
- [ ] Output conventional commit message for user
