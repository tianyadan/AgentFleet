package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/internal/service"
	"colleague-avatar/server/internal/store"
)

// AdminWorkflowsList 编排列表（附带最新运行状态）。
func (h *Handler) AdminWorkflowsList(c *gin.Context) {
	items, err := h.svc.Store.ListWorkflowDefinitions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ids := make([]int64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	runs, _ := h.svc.Store.ListLatestRunsByDefinitions(c.Request.Context(), ids)
	type row struct {
		store.WorkflowDefinition
		LatestRun *store.WorkflowRun `json:"latest_run,omitempty"`
	}
	out := make([]row, 0, len(items))
	for _, it := range items {
		r := row{WorkflowDefinition: it}
		if lr, ok := runs[it.ID]; ok {
			r.LatestRun = lr
		}
		out = append(out, r)
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// AdminWorkflowsCreate 新建编排。
func (h *Handler) AdminWorkflowsCreate(c *gin.Context) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		GraphJSON   string `json:"graph_json"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	d, err := h.svc.CreateWorkflow(c.Request.Context(), body.Name, body.Description, body.GraphJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, d)
}

// AdminWorkflowsGet 详情。
func (h *Handler) AdminWorkflowsGet(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	d, err := h.svc.Store.GetWorkflowDefinition(c.Request.Context(), id)
	if err != nil || d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	runs, _ := h.svc.Store.ListWorkflowRuns(c.Request.Context(), id, 20)
	c.JSON(http.StatusOK, gin.H{"definition": d, "runs": runs})
}

// AdminWorkflowsUpdate 更新。
func (h *Handler) AdminWorkflowsUpdate(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		GraphJSON   string `json:"graph_json"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	d, err := h.svc.UpdateWorkflow(c.Request.Context(), id, body.Name, body.Description, body.GraphJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, d)
}

// AdminWorkflowsDelete 删除。
func (h *Handler) AdminWorkflowsDelete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.DeleteWorkflow(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminWorkflowsStart 启动运行。
func (h *Handler) AdminWorkflowsStart(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		InputPrompt string `json:"input_prompt"`
	}
	_ = c.ShouldBindJSON(&body)
	run, err := h.svc.StartWorkflowRun(c.Request.Context(), id, strings.TrimSpace(body.InputPrompt))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, run)
}

// AdminWorkflowRunGet 运行详情。
func (h *Handler) AdminWorkflowRunGet(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("runId"), 10, 64)
	detail, err := h.svc.GetWorkflowRunDetail(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

// AdminWorkflowRunStop 终止。
func (h *Handler) AdminWorkflowRunStop(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("runId"), 10, 64)
	if err := h.svc.StopWorkflowRun(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminWorkflowRunRetry 从失败节点重试。
func (h *Handler) AdminWorkflowRunRetry(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("runId"), 10, 64)
	if err := h.svc.RetryWorkflowRun(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminWorkflowNodeRetry 按节点重试；body.extra_prompt 非空则为「修改要求并重新执行」。
func (h *Handler) AdminWorkflowNodeRetry(c *gin.Context) {
	runID, _ := strconv.ParseInt(c.Param("runId"), 10, 64)
	nodeID := c.Param("nodeId")
	var body struct {
		ExtraPrompt string `json:"extra_prompt"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := h.svc.RetryWorkflowNode(c.Request.Context(), runID, nodeID, body.ExtraPrompt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminWorkflowRunResume 自检恢复 interrupted / waiting_recovery 工作流。
func (h *Handler) AdminWorkflowRunResume(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("runId"), 10, 64)
	if err := h.svc.ResumeWorkflowRun(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminWorkflowRunReview 人工审核。
func (h *Handler) AdminWorkflowRunReview(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("runId"), 10, 64)
	var body struct {
		Action       string `json:"action"`
		RejectPrompt string `json:"reject_prompt"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Action == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action required"})
		return
	}
	if err := h.svc.ReviewWorkflowRun(c.Request.Context(), id, body.Action, body.RejectPrompt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminWorkflowPermissionPending 团队编排进行中节点的待授权列表。
func (h *Handler) AdminWorkflowPermissionPending(c *gin.Context) {
	ids, err := h.svc.Store.ListRunningNodeConversationIDs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	all := h.svc.Perms.Pending(0)
	out := make([]interface{}, 0)
	for _, p := range all {
		if want[p.ConvID] {
			out = append(out, p)
		}
	}
	running, _ := h.svc.Store.CountActiveWorkflowRuns(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"items": out, "running_teams": running})
}

// OccupancyBusyJSON 统一忙碌错误响应。
func occupancyBusyJSON(c *gin.Context, err error) bool {
	if be, ok := err.(*service.OccupancyBusyError); ok {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "occupancy": be.Occ})
		return true
	}
	return false
}
