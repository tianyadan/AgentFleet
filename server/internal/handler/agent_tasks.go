package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/internal/store"
)

// AgentTaskReport 接收某 agent 上报的任务进度(可插拔: agent_type 区分来源)。
// body: {agent_type, agent_id, task_id, task_name, status, progress, message, payload}
func (h *Handler) AgentTaskReport(c *gin.Context) {
	var r store.AgentTaskReport
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if r.AgentType == "" || r.TaskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_type and task_id required"})
		return
	}
	if r.Status == "" {
		r.Status = "running"
	}
	if _, err := h.svc.Store.UpsertAgentTask(c.Request.Context(), r); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AgentTaskList 查询任务进度。query: agent_type? status?
func (h *Handler) AgentTaskList(c *gin.Context) {
	list, err := h.svc.Store.AgentTasks(c.Request.Context(), c.Query("agent_type"), c.Query("status"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": list})
}

// AgentTaskTypes 返回出现过的 agent_type(可插拔分组)。
func (h *Handler) AgentTaskTypes(c *gin.Context) {
	types, err := h.svc.Store.AgentTaskTypes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": types})
}
