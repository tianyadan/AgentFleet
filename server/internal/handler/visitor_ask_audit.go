package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/internal/store"
)

// AdminVisitorAskLogs 前台 Ask 审计分页列表。
func (h *Handler) AdminVisitorAskLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", c.DefaultQuery("page_size", "20")))
	agentID, _ := strconv.ParseInt(c.DefaultQuery("agentId", c.DefaultQuery("agent_id", "0")), 10, 64)
	f := store.VisitorAskLogFilter{
		Page:        page,
		PageSize:    pageSize,
		VisitorName: strings.TrimSpace(c.Query("visitorName")),
		AgentID:     agentID,
		Status:      strings.TrimSpace(c.Query("status")),
		Keyword:     strings.TrimSpace(c.Query("keyword")),
		BeginTime:   strings.TrimSpace(c.Query("beginTime")),
		EndTime:     strings.TrimSpace(c.Query("endTime")),
	}
	if f.VisitorName == "" {
		f.VisitorName = strings.TrimSpace(c.Query("visitor_name"))
	}
	if f.BeginTime == "" {
		f.BeginTime = strings.TrimSpace(c.Query("begin_time"))
	}
	if f.EndTime == "" {
		f.EndTime = strings.TrimSpace(c.Query("end_time"))
	}
	items, total, err := h.svc.Store.ListVisitorAskLogs(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		out = append(out, gin.H{
			"id":             it.ID,
			"visitorId":      it.VisitorID,
			"visitorName":    it.VisitorName,
			"conversationId": it.ConversationID,
			"agentId":        it.AgentID,
			"agentName":      it.AgentName,
			"question":       it.Question,
			"inputTokens":    it.InputTokens,
			"outputTokens":   it.OutputTokens,
			"cachedTokens":   it.CachedTokens,
			"totalTokens":    it.TotalTokens,
			"durationMs":     it.DurationMs,
			"status":         it.Status,
			"ipMasked":       store.MaskIP(it.IP),
			"createdAt":      it.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"items": out, "total": total, "page": f.Page, "pageSize": f.PageSize,
	})
}

// AdminVisitorAskLogDetail 审计详情。
func (h *Handler) AdminVisitorAskLogDetail(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	d, err := h.svc.Store.GetVisitorAskLogDetail(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                 d.ID,
		"visitorId":          d.VisitorID,
		"visitorName":        d.VisitorName,
		"visitorPublicId":    d.VisitorPublicID,
		"conversationId":     d.ConversationID,
		"agentId":            d.AgentID,
		"agentName":          d.AgentName,
		"engine":             d.AgentEngine,
		"ip":                 d.IP,
		"ipMasked":           store.MaskIP(d.IP),
		"userAgent":          d.UserAgent,
		"userMessageId":      d.UserMessageID,
		"assistantMessageId": d.AssistantMessageID,
		"question":           d.Question,
		"answer":             d.Answer,
		"inputTokens":        d.InputTokens,
		"outputTokens":       d.OutputTokens,
		"cachedTokens":       d.CachedTokens,
		"totalTokens":        d.TotalTokens,
		"durationMs":         d.DurationMs,
		"status":             d.Status,
		"errorMessage":       d.ErrorMessage,
		"createdAt":          d.CreatedAt,
		"finishedAt":         d.FinishedAt,
	})
}
