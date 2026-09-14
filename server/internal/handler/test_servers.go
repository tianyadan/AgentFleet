package handler

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/internal/agent"
	"colleague-avatar/server/internal/testsrv"
)

func (h *Handler) requireTestServerIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := h.clientIP(c)
		ok, err := h.svc.Store.IsTestServerIPAllowed(c.Request.Context(), ip)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden: IP not in test-server allowlist", "ip": ip})
			return
		}
		c.Next()
	}
}

func (h *Handler) TestServersList(c *gin.Context) {
	items, err := h.svc.Store.ListTestServers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) TestServersCreate(c *gin.Context) {
	var body struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Host) == "" || body.Username == "" || body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host/username/password required"})
		return
	}
	enc, err := testsrv.EncryptPassword(h.svc.Cfg.SecretKey, body.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	id, err := h.svc.Store.InsertTestServer(c.Request.Context(), body.Name, body.Host, body.Port, body.Username, enc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *Handler) TestServersUpdate(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	enc := ""
	updatePass := strings.TrimSpace(body.Password) != ""
	if updatePass {
		var err error
		enc, err = testsrv.EncryptPassword(h.svc.Cfg.SecretKey, body.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := h.svc.Store.UpdateTestServer(c.Request.Context(), id, body.Name, body.Host, body.Port, body.Username, enc, updatePass); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) TestServersDelete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Store.DeleteTestServer(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) TestServerIPsList(c *gin.Context) {
	items, err := h.svc.Store.ListTestServerAllowedIPs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) TestServerIPsAdd(c *gin.Context) {
	var body struct {
		IP   string `json:"ip"`
		Note string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.IP) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip required"})
		return
	}
	id, err := h.svc.Store.UpsertTestServerAllowedIP(c.Request.Context(), strings.TrimSpace(body.IP), body.Note)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *Handler) TestServerIPsDelete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Store.DeleteTestServerAllowedIP(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) TestServerPreview(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		Action string                 `json:"action"` // containers | logs
		Args   map[string]interface{} `json:"args"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	cmd, err := testsrv.BuildCommand(body.Action, body.Args)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ex := testsrv.ExplainCommand(c.Request.Context(), h.svc.Cfg.ClaudeBin, agent.CleanEnv(os.Environ()), cmd, h.svc.Cfg.ReviewTimeout())
	// 命令已由模板白名单约束为 docker ps/logs;翻译失败时仍允许人工确认执行
	if ex.Err != nil {
		if ex.Meaning == "" || ex.Meaning == "翻译不可用" {
			ex.Meaning = "翻译不可用(模板只读命令)"
		}
		if ex.Risk == "" {
			ex.Risk = "low"
		}
		ex.Harmless = true
	}
	pending := h.svc.TestHub.Register(id, body.Action, cmd, ex.Meaning, ex.Risk, ex.Harmless, h.clientIP(c))
	c.JSON(http.StatusOK, gin.H{
		"request_id":  pending.ID,
		"server_id":   id,
		"command":     pending.Command,
		"meaning":     pending.Meaning,
		"risk":        pending.Risk,
		"harmless":    pending.Harmless,
		"deadline":    pending.Deadline,
		"explain_err": errString(ex.Err),
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (h *Handler) TestServerDecide(c *gin.Context) {
	var body struct {
		RequestID string `json:"request_id"`
		Behavior  string `json:"behavior"` // allow / deny
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.RequestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request_id required"})
		return
	}
	p, ok := h.svc.TestHub.Take(body.RequestID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "request expired or not found"})
		return
	}
	if p.ClientIP != h.clientIP(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "ip mismatch"})
		return
	}
	if body.Behavior != "allow" {
		c.JSON(http.StatusOK, gin.H{"behavior": "deny", "stdout": "", "stderr": ""})
		return
	}
	if p.Risk == "high" || !p.Harmless {
		c.JSON(http.StatusForbidden, gin.H{"error": "服务端拒绝:风险过高或未判定无害", "risk": p.Risk, "harmless": p.Harmless, "command": p.Command})
		return
	}
	host, port, user, passEnc, err := h.svc.Store.GetTestServerSecret(c.Request.Context(), p.ServerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pass, err := testsrv.DecryptPassword(h.svc.Cfg.SecretKey, passEnc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decrypt password: " + err.Error()})
		return
	}
	stdout, stderr, err := testsrv.RunSSH(host, port, user, pass, p.Command, h.svc.Cfg.SSHInsecureIgnoreHostKey)
	resp := gin.H{"behavior": "allow", "command": p.Command, "stdout": stdout, "stderr": stderr}
	if err != nil {
		resp["error"] = err.Error()
	}
	c.JSON(http.StatusOK, resp)
}
