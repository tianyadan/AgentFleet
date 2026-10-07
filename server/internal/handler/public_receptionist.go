package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/internal/store"
	"colleague-avatar/server/internal/visitorcookie"
)

// PublicReceptionist GET /api/public/receptionist
func (h *Handler) PublicReceptionist(c *gin.Context) {
	out, err := h.svc.GetReceptionistPublic(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// PublicVisitorMe GET /api/public/visitor/me
func (h *Handler) PublicVisitorMe(c *gin.Context) {
	pid, ok := visitorcookie.FromRequest(c.Request, h.svc.Cfg.JWTSecret)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"registered": false})
		return
	}
	v, err := h.svc.Store.GetVisitorByPublicID(c.Request.Context(), pid)
	if err != nil || v == nil {
		c.JSON(http.StatusOK, gin.H{"registered": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"registered": true, "name": v.Name})
}

// PublicVisitorRegister POST /api/public/visitor/register
func (h *Handler) PublicVisitorRegister(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写来访者姓名"})
		return
	}
	ctx := c.Request.Context()
	if pid, ok := visitorcookie.FromRequest(c.Request, h.svc.Cfg.JWTSecret); ok {
		if v, _ := h.svc.Store.GetVisitorByPublicID(ctx, pid); v != nil {
			_ = h.svc.Store.UpdateVisitorName(ctx, v.ID, name)
			visitorcookie.Set(c.Writer, h.svc.Cfg.JWTSecret, v.PublicID)
			c.JSON(http.StatusOK, gin.H{"registered": true, "name": name})
			return
		}
	}
	v, err := h.svc.Store.CreateVisitor(ctx, name)
	if err != nil || v == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登记失败"})
		return
	}
	visitorcookie.Set(c.Writer, h.svc.Cfg.JWTSecret, v.PublicID)
	c.JSON(http.StatusOK, gin.H{"registered": true, "name": v.Name})
}

// PublicVisitorMessages GET /api/public/messages
func (h *Handler) PublicVisitorMessages(c *gin.Context) {
	v, ok := h.requireVisitor(c)
	if !ok {
		return
	}
	if v.CurrentConversationID <= 0 {
		c.JSON(http.StatusOK, gin.H{"items": []any{}, "conversation_id": 0})
		return
	}
	items, err := h.svc.Store.Messages(c.Request.Context(), v.CurrentConversationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "conversation_id": v.CurrentConversationID})
}

// PublicNewConversation POST /api/public/conversations/new
func (h *Handler) PublicNewConversation(c *gin.Context) {
	v, ok := h.requireVisitor(c)
	if !ok {
		return
	}
	recv, err := h.svc.Store.GetReceptionist(c.Request.Context())
	if err != nil || recv == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "暂未设置前台助理", "code": "no_receptionist"})
		return
	}
	convID, err := h.svc.NewVisitorConversation(c.Request.Context(), v, recv.ID, h.clientIP(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	visitorcookie.Set(c.Writer, h.svc.Cfg.JWTSecret, v.PublicID)
	c.JSON(http.StatusOK, gin.H{"conversation_id": convID})
}

// PublicAsk POST /api/public/ask — SSE；前端用 fetch + ReadableStream 解析
func (h *Handler) PublicAsk(c *gin.Context) {
	v, ok := h.requireVisitor(c)
	if !ok {
		return
	}
	var body struct {
		Question string `json:"question"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Question) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty question"})
		return
	}
	recv, err := h.svc.Store.GetReceptionist(c.Request.Context())
	if err != nil || recv == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "暂未设置前台助理", "code": "no_receptionist"})
		return
	}
	convID, err := h.svc.EnsureVisitorConversation(c.Request.Context(), v, recv.ID, h.clientIP(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, okf := c.Writer.(http.Flusher)
	if !okf {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}
	var wmu sync.Mutex
	sendEvent := func(payload any) {
		wmu.Lock()
		defer wmu.Unlock()
		b, _ := json.Marshal(payload)
		_, _ = c.Writer.Write([]byte("data: " + string(b) + "\n\n"))
		flusher.Flush()
	}
	sendEvent(map[string]any{"type": "start", "conversation_id": convID, "agent_id": recv.ID})

	ip := h.clientIP(c)
	ua := c.Request.UserAgent()
	msg, askErr := h.svc.AskReceptionist(c.Request.Context(), v.ID, recv.ID, convID, body.Question, ip, ua, func(ev map[string]any) {
		if ev == nil {
			return
		}
		if t, _ := ev["type"].(string); t == "chunk" {
			if ctn, ok := ev["content"].(string); ok {
				ev["content"] = strings.ReplaceAll(ctn, "\n", "\\n")
			}
		}
		if t, _ := ev["type"].(string); t == "permission_request" {
			return
		}
		sendEvent(ev)
	})
	visitorcookie.Set(c.Writer, h.svc.Cfg.JWTSecret, v.PublicID)
	if askErr != nil {
		sendEvent(map[string]any{"type": "error", "message": askErr.Error()})
	}
	if msg != nil {
		sendEvent(map[string]any{
			"type": "done", "conversation_id": convID, "status": msg.Status,
			"content": msg.Content, "reply_ms": msg.ReplyMs,
		})
	} else {
		sendEvent(map[string]any{"type": "done", "conversation_id": convID})
	}
}

// PublicContext GET /api/public/context
func (h *Handler) PublicContext(c *gin.Context) {
	recv, err := h.svc.Store.GetReceptionist(c.Request.Context())
	if err != nil || recv == nil {
		c.JSON(http.StatusOK, gin.H{"used_tokens": 0, "window_tokens": 0, "estimated": true, "used_percent": 0})
		return
	}
	// 前台占用展示：用访客当前会话 meta，而非数字人主会话
	v, vok := h.optionalVisitor(c)
	if vok && v.CurrentConversationID > 0 {
		meta, _ := h.svc.Store.GetConversationEngineMeta(c.Request.Context(), v.CurrentConversationID)
		win := recv.ContextWindowTokens
		pct := 0.0
		if win > 0 {
			pct = float64(meta.UsedTokens) * 100 / float64(win)
		}
		c.JSON(http.StatusOK, gin.H{
			"used_tokens": meta.UsedTokens, "window_tokens": win,
			"estimated": true, "used_percent": pct,
		})
		return
	}
	ec, err := h.svc.FetchEngineContext(c.Request.Context(), recv.ID, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"used_tokens": 0, "window_tokens": 0, "estimated": true, "used_percent": 0})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"used_tokens": ec.UsedTokens, "window_tokens": recv.ContextWindowTokens,
		"estimated": true, "used_percent": ec.UsedPercent,
	})
}

func (h *Handler) requireVisitor(c *gin.Context) (*store.Visitor, bool) {
	pid, ok := visitorcookie.FromRequest(c.Request, h.svc.Cfg.JWTSecret)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登记来访者姓名", "code": "visitor_required"})
		return nil, false
	}
	v, err := h.svc.Store.GetVisitorByPublicID(c.Request.Context(), pid)
	if err != nil || v == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登记来访者姓名", "code": "visitor_required"})
		return nil, false
	}
	return v, true
}

func (h *Handler) optionalVisitor(c *gin.Context) (*store.Visitor, bool) {
	pid, ok := visitorcookie.FromRequest(c.Request, h.svc.Cfg.JWTSecret)
	if !ok {
		return nil, false
	}
	v, err := h.svc.Store.GetVisitorByPublicID(c.Request.Context(), pid)
	if err != nil || v == nil {
		return nil, false
	}
	return v, true
}
