package handler

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/internal/store"
)

// AdminAgentsList 智能体列表。
func (h *Handler) AdminAgentsList(c *gin.Context) {
	items, err := h.svc.Store.ListManagedAgents(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []store.ManagedAgent{}
	}
	occMap, _ := h.svc.Store.ListOccupancyMap(c.Request.Context())
	type row struct {
		store.ManagedAgent
		Occupancy *store.Occupancy `json:"occupancy,omitempty"`
	}
	out := make([]row, 0, len(items))
	for _, a := range items {
		r := row{ManagedAgent: a}
		if o, ok := occMap[a.ID]; ok {
			oc := o
			r.Occupancy = &oc
		}
		out = append(out, r)
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// AdminAgentsCreate 一键创建智能体。
func (h *Handler) AdminAgentsCreate(c *gin.Context) {
	var body struct {
		Name            string `json:"name"`
		Engine          string `json:"engine"`
		BinPath         string `json:"bin_path"`
		RulesPrompt     string `json:"rules_prompt"`
		AllowWrite      *bool  `json:"allow_write"`
		AllowNetwork    *bool  `json:"allow_network"`
		AllowRm         *bool  `json:"allow_rm"`
		AllowBrowser    *bool  `json:"allow_browser"`
		WorkspacePath   string `json:"workspace_path"`
		ScheduleEnabled bool   `json:"schedule_enabled"`
		ScheduleCron    string `json:"schedule_cron"`
		ScheduleLabel   string `json:"schedule_label"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	name := strings.TrimSpace(body.Name)
	engine := strings.ToLower(strings.TrimSpace(body.Engine))
	if name == "" {
		name = "未命名智能体"
	}
	if !store.ValidEngine(engine) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "engine must be claude|codex|agent"})
		return
	}
	ws := strings.TrimSpace(body.WorkspacePath)
	se := body.ScheduleEnabled
	cron := strings.TrimSpace(body.ScheduleCron)
	label := strings.TrimSpace(body.ScheduleLabel)
	in := store.ManagedAgentInput{
		Name: name, Engine: engine, BinPath: strings.TrimSpace(body.BinPath), RulesPrompt: body.RulesPrompt,
		AllowWrite: body.AllowWrite, AllowNetwork: body.AllowNetwork, AllowRm: body.AllowRm, AllowBrowser: body.AllowBrowser,
		WorkspacePath: &ws, ScheduleEnabled: &se, ScheduleCron: &cron, ScheduleLabel: &label,
	}
	id, err := h.svc.Store.CreateManagedAgent(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	a, _ := h.svc.Store.GetManagedAgent(c.Request.Context(), id)
	if h.svc.Sched != nil {
		h.svc.Sched.Reload(c.Request.Context())
	}
	c.JSON(http.StatusOK, a)
}

// AdminAgentsUpdate 重命名 / 改规则 / 改 bin / 权限策略。
func (h *Handler) AdminAgentsUpdate(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	raw, _ := io.ReadAll(c.Request.Body)
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	in := store.ManagedAgentInput{}
	if v, ok := body["name"]; ok {
		var s string
		_ = json.Unmarshal(v, &s)
		in.Name = s
	}
	if v, ok := body["bin_path"]; ok {
		var s string
		_ = json.Unmarshal(v, &s)
		in.BinPath = s
	}
	if v, ok := body["rules_prompt"]; ok {
		var s string
		_ = json.Unmarshal(v, &s)
		in.RulesPrompt = s
	}
	parseBoolPtr := func(key string) *bool {
		v, ok := body[key]
		if !ok {
			return nil
		}
		var b bool
		if json.Unmarshal(v, &b) != nil {
			return nil
		}
		return &b
	}
	parseStrPtr := func(key string) *string {
		v, ok := body[key]
		if !ok {
			return nil
		}
		var s string
		if json.Unmarshal(v, &s) != nil {
			return nil
		}
		return &s
	}
	in.AllowWrite = parseBoolPtr("allow_write")
	in.AllowNetwork = parseBoolPtr("allow_network")
	in.AllowRm = parseBoolPtr("allow_rm")
	in.AllowBrowser = parseBoolPtr("allow_browser")
	in.WorkspacePath = parseStrPtr("workspace_path")
	in.ScheduleEnabled = parseBoolPtr("schedule_enabled")
	in.ScheduleCron = parseStrPtr("schedule_cron")
	in.ScheduleLabel = parseStrPtr("schedule_label")

	if err := h.svc.Store.UpdateManagedAgent(c.Request.Context(), id, in); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.svc.Sched != nil {
		h.svc.Sched.Reload(c.Request.Context())
	}
	a, _ := h.svc.Store.GetManagedAgent(c.Request.Context(), id)
	c.JSON(http.StatusOK, a)
}

// AdminAgentsDelete 删除智能体。
func (h *Handler) AdminAgentsDelete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Store.DeleteManagedAgent(c.Request.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.svc.Sched != nil {
		h.svc.Sched.Reload(c.Request.Context())
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminAgentsMessages 当前对话消息(分页:默认最新 20 条;before_id 上翻更早)。
func (h *Handler) AdminAgentsMessages(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	a, err := h.svc.Store.GetManagedAgent(c.Request.Context(), id)
	if err != nil || a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if a.ConversationID == 0 {
		c.JSON(http.StatusOK, gin.H{"items": []store.Message{}, "conversation_id": 0, "total": 0, "has_more": false})
		return
	}
	beforeID, _ := strconv.ParseInt(c.DefaultQuery("before_id", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, total, hasMore, err := h.svc.Store.MessagesPage(c.Request.Context(), a.ConversationID, beforeID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []store.Message{}
	}
	c.JSON(http.StatusOK, gin.H{
		"items": items, "conversation_id": a.ConversationID,
		"total": total, "has_more": hasMore, "limit": limit,
	})
}

// AdminAgentsFile 读取智能体工作区内的图片(鉴权;防目录穿越)。
func (h *Handler) AdminAgentsFile(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	raw := strings.TrimSpace(c.Query("path"))
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}
	a, err := h.svc.Store.GetManagedAgent(c.Request.Context(), id)
	if err != nil || a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	abs, ct, err := h.svc.ResolveAgentImage(a, raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Header("Cache-Control", "private, max-age=60")
	c.Header("Content-Type", ct)
	c.File(abs)
}

// AdminAgentsAsk SSE 提问管理型智能体(含授权事件转发)。
func (h *Handler) AdminAgentsAsk(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		Question string `json:"question"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Question) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "question required"})
		return
	}

	// 先确保会话存在,便于按 conversation_id 过滤授权事件
	a, err := h.svc.Store.GetManagedAgent(c.Request.Context(), id)
	if err != nil || a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	convID := a.ConversationID
	if convID == 0 {
		convID, err = h.svc.Store.CreateAgentConversation(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_ = h.svc.Store.SetManagedAgentConversation(c.Request.Context(), id, convID)
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
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

	sub := h.svc.Perms.Subscribe()
	defer h.svc.Perms.Unsubscribe(sub)
	done := make(chan struct{})
	var fwd sync.WaitGroup
	fwd.Add(1)
	go func() {
		defer fwd.Done()
		for {
			select {
			case <-done:
				return
			case ev := <-sub:
				var payload map[string]interface{}
				if (ev.Kind == "request" || ev.Kind == "auto") && ev.Req != nil {
					if ev.Req.ConvID != convID {
						continue
					}
					typ := "permission_request"
					if ev.Kind == "auto" {
						typ = "permission_auto"
					}
					payload = map[string]interface{}{
						"type": typ, "request_id": ev.Req.ID, "tool_name": ev.Req.ToolName,
						"summary": ev.Req.Summary, "deadline": ev.Req.Deadline, "note": ev.Req.Note,
						"meaning": ev.Req.Meaning, "risk": ev.Req.Risk, "conversation_id": convID,
					}
				} else if ev.Kind == "command" && ev.Req != nil {
					if ev.Req.ConvID != 0 && ev.Req.ConvID != convID {
						continue
					}
					payload = map[string]interface{}{
						"type": "command", "tool_name": ev.Req.ToolName, "summary": ev.Req.Summary,
						"decision": ev.Req.Decision, "decided_by": ev.Req.DecidedBy,
						"risk": ev.Req.Risk, "meaning": ev.Req.Meaning,
					}
					// 同步一条精简 activity,便于前端活动条展示
					sendEvent(map[string]interface{}{
						"type": "activity", "tool": ev.Req.ToolName,
						"summary": activitySummary(ev.Req.ToolName, ev.Req.Summary),
					})
				} else if ev.Kind == "resolved" && ev.Res != nil {
					payload = map[string]interface{}{"type": "permission_resolved", "request_id": ev.Res.ID, "behavior": ev.Res.Behavior, "by": ev.Res.By}
				} else {
					continue
				}
				sendEvent(payload)
			}
		}
	}()
	var stopOnce sync.Once
	stopForward := func() {
		stopOnce.Do(func() { close(done); fwd.Wait() })
	}
	defer stopForward()

	msg, askErr := h.svc.AskManaged(c.Request.Context(), id, body.Question, func(ev map[string]any) {
		if ev == nil {
			return
		}
		if t, _ := ev["type"].(string); t == "chunk" {
			if c, ok := ev["content"].(string); ok {
				ev["content"] = strings.ReplaceAll(c, "\n", "\\n")
			}
		}
		sendEvent(ev)
	})
	stopForward()
	if askErr != nil {
		sendEvent(map[string]interface{}{"type": "error", "message": askErr.Error()})
		return
	}
	sendEvent(map[string]interface{}{
		"type": "done", "conversation_id": msg.ConversationID,
		"reply_ms": msg.ReplyMs, "status": msg.Status,
	})
}

// AdminAgentsClear 清空对话。
func (h *Handler) AdminAgentsClear(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.ClearManagedChat(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminAgentsNew 新对话。
func (h *Handler) AdminAgentsNew(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	conv, err := h.svc.NewManagedChat(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversation_id": conv})
}

// AdminAgentsCompress 压缩上下文。
func (h *Handler) AdminAgentsCompress(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.CompressManagedChat(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminAgentsTasks 该智能体相关任务进度(按 agent_id；优先 managed 类型平台任务)。
func (h *Handler) AdminAgentsTasks(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	a, err := h.svc.Store.GetManagedAgent(c.Request.Context(), id)
	if err != nil || a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	aid := strconv.FormatInt(a.ID, 10)
	items, err := h.svc.Store.AgentTasksByAgentID(c.Request.Context(), "managed", aid, "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"items": []store.AgentTask{}, "warning": err.Error()})
		return
	}
	if len(items) == 0 {
		items, err = h.svc.Store.AgentTasksByAgentID(c.Request.Context(), "", aid, "")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"items": []store.AgentTask{}, "warning": err.Error()})
			return
		}
	}
	if items == nil {
		items = []store.AgentTask{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// AdminAgentsStop 终止当前进行中的智能体问答。
func (h *Handler) AdminAgentsStop(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	ok := h.svc.StopManaged(id)
	c.JSON(http.StatusOK, gin.H{"ok": true, "stopped": ok})
}

// AdminAgentsCommandAudits 命令审批审计列表。
func (h *Handler) AdminAgentsCommandAudits(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	a, err := h.svc.Store.GetManagedAgent(c.Request.Context(), id)
	if err != nil || a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	items, err := h.svc.Store.ListCommandAudits(c.Request.Context(), a.ConversationID, a.ID, 50)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"items": []store.CommandAudit{}, "warning": err.Error()})
		return
	}
	if items == nil {
		items = []store.CommandAudit{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// AdminNotifyPermission 前端在未查看该 agent 时请求 Bark 推送(策略 A)。
func (h *Handler) AdminNotifyPermission(c *gin.Context) {
	var body struct {
		RequestID  string `json:"request_id"`
		AgentID    int64  `json:"agent_id"`
		AgentName  string `json:"agent_name"`
		ToolName   string `json:"tool_name"`
		Command    string `json:"command"`
		Risk       string `json:"risk"`
		Meaning    string `json:"meaning"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.RequestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request_id required"})
		return
	}
	name := strings.TrimSpace(body.AgentName)
	if name == "" && body.AgentID > 0 {
		if a, _ := h.svc.Store.GetManagedAgent(c.Request.Context(), body.AgentID); a != nil {
			name = a.Name
		}
	}
	if name == "" {
		name = "未命名智能体"
	}
	cmd := strings.TrimSpace(body.Command)
	if cmd == "" {
		cmd = body.ToolName
	}
	risk := strings.TrimSpace(body.Risk)
	if risk == "" {
		risk = "未知"
	}
	riskLabel := map[string]string{"low": "低", "mid": "中", "high": "高", "未知": "未知"}[risk]
	if riskLabel == "" {
		riskLabel = risk
	}
	msg := "智能体: " + name + "\n待执行命令: " + cmd + "\n风险等级: " + riskLabel
	if m := strings.TrimSpace(body.Meaning); m != "" {
		msg += "\n含义/风险说明: " + m
	}
	sent := false
	if h.svc.Notify != nil {
		sent = h.svc.Notify.PermissionOnce(body.RequestID, msg)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "sent": sent})
}

// activitySummary 把授权命令压成活动条文案(不展开长 diff)。
func activitySummary(tool, summary string) string {
	tool = strings.TrimSpace(tool)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		if tool == "" {
			return "正在调用工具…"
		}
		return "调用 " + tool + "…"
	}
	runes := []rune(summary)
	if len(runes) > 72 {
		summary = string(runes[:72]) + "…"
	}
	low := strings.ToLower(tool)
	if low == "bash" || low == "shell" {
		return "执行命令 · " + summary
	}
	return summary
}

// AdminAgentFoldersList 文件夹列表。
func (h *Handler) AdminAgentFoldersList(c *gin.Context) {
	items, err := h.svc.Store.ListAgentFolders(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []store.AgentFolder{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// AdminAgentFoldersCreate 创建空文件夹。
func (h *Handler) AdminAgentFoldersCreate(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&body)
	id, err := h.svc.Store.CreateAgentFolder(c.Request.Context(), body.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "name": strings.TrimSpace(body.Name)})
}

// AdminAgentFoldersUpdate 重命名文件夹。
func (h *Handler) AdminAgentFoldersUpdate(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.Store.RenameAgentFolder(c.Request.Context(), id, body.Name); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminAgentFoldersDelete 删除文件夹。
func (h *Handler) AdminAgentFoldersDelete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Store.DeleteAgentFolder(c.Request.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminAgentsSetFolder 拖拽归类：将智能体移入文件夹（folder_id=0 表示根目录）。
func (h *Handler) AdminAgentsSetFolder(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var body struct {
		FolderID int64 `json:"folder_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.Store.SetManagedAgentFolder(c.Request.Context(), id, body.FolderID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	a, _ := h.svc.Store.GetManagedAgent(c.Request.Context(), id)
	c.JSON(http.StatusOK, a)
}

// AdminAgentsStatus 本地状态快照（不入库）；优先引擎真实上下文。
func (h *Handler) AdminAgentsStatus(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	text, err := h.svc.BuildAgentStatusReport(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"text": text})
}

// AdminAgentsContext 引擎真实上下文占用（圆环 / 轮询）；fresh=1 强制探测。
func (h *Handler) AdminAgentsContext(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	fresh := c.Query("fresh") == "1" || c.Query("fresh") == "true"
	ctx, err := h.svc.FetchEngineContext(c.Request.Context(), id, fresh)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ctx)
}

