package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"colleague-avatar/server/internal/auth"
	"colleague-avatar/server/internal/permission"
	"colleague-avatar/server/internal/service"
	"colleague-avatar/server/internal/store"
)

// Handler 收集路由处理函数。
type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// clientIP 返回客户端 IP,用于会话分组。采用简单方案:直接用 c.ClientIP(),不信任代理头,
// 保证分组与 IP 白名单一致(同一局域网内经 vite 代理访问时归为一组 127.0.0.1)。
func (h *Handler) clientIP(c *gin.Context) string {
	return c.ClientIP()
}

// Register 挂载路由。公开对话接口与需 JWT 的管理接口分组。
func (h *Handler) Register(r *gin.Engine) {
	api := r.Group("/api")
	{
		// 认证
		api.POST("/auth/login", h.Login)
		api.POST("/auth/logout", h.Logout)
		api.GET("/auth/me", auth.RequireAdminJWT(h.svc.Cfg.JWTSecret), h.Me)

		// 公开:对话必需
		api.POST("/question", h.Question)
		api.POST("/conversations", h.CreateConversation)
		api.POST("/conversations/:id/compress", h.ConversationCompress)
		api.GET("/workspaces", h.Workspaces)
		// 多 Agent 上报/查询保持公开(外部 agent 与 E-bot 经 curl 调用)
		api.POST("/agents/tasks", h.AgentTaskReport)
		api.GET("/agents/tasks", h.AgentTaskList)
		api.GET("/agents/task-types", h.AgentTaskTypes)

		// 需 JWT:历史 / 统计 / 命令清单 / 数字员工
		adm := api.Group("")
		adm.Use(auth.RequireAdminJWT(h.svc.Cfg.JWTSecret))
		{
			adm.GET("/conversations", h.Conversations)
			adm.GET("/conversations/:id/messages", h.ConversationMessages)
			adm.GET("/commands", h.Commands)
			adm.GET("/stats", h.Stats)
			adm.GET("/stats/days", h.StatsDays)
			adm.GET("/stats/days/:date", h.StatsDayIP)

			adm.GET("/admin/agents", h.AdminAgentsList)
			adm.POST("/admin/agents", h.AdminAgentsCreate)
			adm.POST("/admin/agents/:id/copy", h.AdminAgentsCopy)
			adm.POST("/admin/agents/:id/retry-init", h.AdminAgentsRetryInit)
			adm.POST("/admin/agents/:id/avatar", h.AdminAgentsUploadAvatar)
			adm.DELETE("/admin/agents/:id/avatar", h.AdminAgentsClearAvatar)
			adm.PATCH("/admin/agents/:id", h.AdminAgentsUpdate)
			adm.DELETE("/admin/agents/:id", h.AdminAgentsDelete)
			adm.GET("/admin/agents/:id/messages", h.AdminAgentsMessages)
			adm.GET("/admin/agents/:id/file", h.AdminAgentsFile)
			adm.GET("/admin/agents/:id/status", h.AdminAgentsStatus)
			adm.GET("/admin/agents/:id/context", h.AdminAgentsContext)
			adm.POST("/admin/agents/:id/folder", h.AdminAgentsSetFolder)
			adm.POST("/admin/agents/:id/ask", h.AdminAgentsAsk)
			adm.POST("/admin/agents/:id/stop", h.AdminAgentsStop)
			adm.POST("/admin/agents/:id/ensure-conversation", h.AdminAgentsEnsureConversation)
			adm.POST("/admin/agents/:id/clear", h.AdminAgentsClear)
			adm.POST("/admin/agents/:id/compress", h.AdminAgentsCompress)
			adm.POST("/admin/agents/:id/task-plan", h.AdminAgentsTaskPlan)
			adm.GET("/admin/agents/:id/tasks", h.AdminAgentsTasks)
			adm.GET("/admin/agents/:id/command-audits", h.AdminAgentsCommandAudits)
			adm.POST("/admin/agents/notify-permission", h.AdminNotifyPermission)

			adm.GET("/admin/agent-folders", h.AdminAgentFoldersList)
			adm.POST("/admin/agent-folders", h.AdminAgentFoldersCreate)
			adm.PATCH("/admin/agent-folders/:id", h.AdminAgentFoldersUpdate)
			adm.DELETE("/admin/agent-folders/:id", h.AdminAgentFoldersDelete)

			adm.GET("/admin/workflows", h.AdminWorkflowsList)
			adm.POST("/admin/workflows", h.AdminWorkflowsCreate)
			adm.GET("/admin/workflows/permission-pending", h.AdminWorkflowPermissionPending)
			adm.GET("/admin/workflows/runs/:runId", h.AdminWorkflowRunGet)
			adm.POST("/admin/workflows/runs/:runId/stop", h.AdminWorkflowRunStop)
			adm.POST("/admin/workflows/runs/:runId/retry", h.AdminWorkflowRunRetry)
			adm.POST("/admin/workflows/runs/:runId/resume", h.AdminWorkflowRunResume)
			adm.POST("/admin/workflows/runs/:runId/nodes/:nodeId/retry", h.AdminWorkflowNodeRetry)
			adm.POST("/admin/workflows/runs/:runId/review", h.AdminWorkflowRunReview)
			adm.GET("/admin/workflows/:id", h.AdminWorkflowsGet)
			adm.PATCH("/admin/workflows/:id", h.AdminWorkflowsUpdate)
			adm.DELETE("/admin/workflows/:id", h.AdminWorkflowsDelete)
			adm.POST("/admin/workflows/:id/runs", h.AdminWorkflowsStart)
		}
	}
	// 测试服务器治理:JWT + 原有 IP 白名单
	ts := r.Group("/api/test-servers")
	ts.Use(auth.RequireAdminJWT(h.svc.Cfg.JWTSecret), h.requireTestServerIP())
	{
		ts.GET("", h.TestServersList)
		ts.POST("", h.TestServersCreate)
		ts.GET("/allowed-ips", h.TestServerIPsList)
		ts.POST("/allowed-ips", h.TestServerIPsAdd)
		ts.DELETE("/allowed-ips/:id", h.TestServerIPsDelete)
		ts.POST("/commands/decide", h.TestServerDecide)
		ts.POST("/:id/preview", h.TestServerPreview)
		ts.PATCH("/:id", h.TestServersUpdate)
		ts.DELETE("/:id", h.TestServersDelete)
	}
	// 数据查询:保持公开(分身 Agent 经 curl 调用,无 JWT)
	dbg := r.Group("/api/db")
	{
		dbg.GET("/databases", h.DBDatabases)
		dbg.GET("/tables", h.DBTables)
		dbg.POST("/query", h.DBQuery)
	}
	// 工具授权代理:公开(对话必需)
	perm := r.Group("/api/permissions")
	{
		perm.POST("/request", h.PermissionRequest)
		perm.GET("/pending", h.PermissionPending)
		perm.POST("/decide", h.PermissionDecide)
		perm.POST("/auto", h.PermissionAuto)
		perm.POST("/compact-sync", h.PermissionCompactSync)
	}
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
}

// CreateConversation 新建会话。body: {mode: single|chat}
func (h *Handler) CreateConversation(c *gin.Context) {
	var body struct {
		Mode string `json:"mode"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Mode == "" {
		body.Mode = service.ModeSingle
	}
	if body.Mode != service.ModeSingle && body.Mode != service.ModeChat {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode"})
		return
	}
	id, err := h.svc.Store.CreateConversation(c.Request.Context(), h.clientIP(c), body.Mode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "mode": body.Mode})
}

// Question 处理提问,SSE 流式返回。req: {question, workspace?, conversation_id?, mode?}
func (h *Handler) Question(c *gin.Context) {
	var req service.QuestionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Question == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "question is required"})
		return
	}
	req.UserIP = h.clientIP(c)

	// 会话归属校验失败(不属于该IP)直接拒绝
	if req.ConversationID != 0 {
		ok, err := h.svc.Store.ConversationExists(c.Request.Context(), req.ConversationID, req.UserIP)
		if err != nil || !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "conversation not accessible"})
			return
		}
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	// sendEvent 串行化所有 SSE 写入(Ask 回调与授权转发可能并发写 c.Writer)。
	var wmu sync.Mutex
	sendEvent := func(payload map[string]interface{}) {
		wmu.Lock()
		defer wmu.Unlock()
		data, _ := json.Marshal(payload)
		c.Writer.WriteString("data: " + string(data) + "\n\n")
		flusher.Flush()
	}
	send := func(typ string, payload map[string]interface{}) {
		payload["type"] = typ
		sendEvent(payload)
	}

	// 订阅授权事件,按本连接的 clientIP 过滤转发到同一 SSE 通道。
	// 挂起授权的清理由:所有浏览器经 vite 代理到达时 clientIP 都是 127.0.0.1,
	// 按 IP 清理会误伤他人请求;清理由 Ask 结束时的 DropByConv 完成。
	myIP := req.UserIP
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
					if ev.Req.ClientIP != myIP {
						continue
					}
					typ := "permission_request"
					if ev.Kind == "auto" {
						typ = "permission_auto" // AI 已直接放行,只展示轨迹
					}
					payload = map[string]interface{}{"type": typ, "request_id": ev.Req.ID, "tool_name": ev.Req.ToolName, "summary": ev.Req.Summary, "deadline": ev.Req.Deadline, "note": ev.Req.Note, "meaning": ev.Req.Meaning, "risk": ev.Req.Risk}
				} else if ev.Kind == "command" && ev.Req != nil {
					payload = map[string]interface{}{
						"type": "command", "tool_name": ev.Req.ToolName, "summary": ev.Req.Summary,
						"decision": ev.Req.Decision, "decided_by": ev.Req.DecidedBy,
						"risk": ev.Req.Risk, "meaning": ev.Req.Meaning,
					}
				} else if ev.Kind == "resolved" && ev.Res != nil {
					payload = map[string]interface{}{"type": "permission_resolved", "request_id": ev.Res.ID, "behavior": ev.Res.Behavior, "by": ev.Res.By}
				} else {
					continue
				}
				sendEvent(payload)
			}
		}
	}()
	// stopForward 必须幂等且在返回前调用:关 done 并等转发 goroutine 彻底退出,
	// 否则 handler 返回、gin 回收 c.Writer 后,goroutine 仍可能 Flush 空缓冲 → panic。
	var stopOnce sync.Once
	stopForward := func() {
		stopOnce.Do(func() {
			close(done)
			fwd.Wait()
		})
	}
	defer stopForward()

	msg, askErr := h.svc.Ask(c.Request.Context(), req, func(chunk string) {
		chunk = strings.ReplaceAll(chunk, "\n", "\\n")
		send("chunk", map[string]interface{}{"content": chunk})
	})

	stopForward() // 先停转发,保证后续对 c.Writer 的写入独占

	if askErr != nil {
		send("error", map[string]interface{}{"message": askErr.Error()})
		return
	}
	if msg != nil && msg.Status == "rejected" {
		send("rejected", map[string]interface{}{"reason": "workspace not authorized"})
		send("done", map[string]interface{}{"id": msg.ID, "conversation_id": msg.ConversationID, "status": "rejected"})
		return
	}
	if msg == nil {
		send("error", map[string]interface{}{"message": "empty response"})
		return
	}
	send("done", map[string]interface{}{"id": msg.ID, "conversation_id": msg.ConversationID, "status": msg.Status, "reply_ms": msg.ReplyMs})
}

// Conversations 分页返回会话列表(默认每页 10,按 updated_at 倒序)。
// query: page, page_size, user_ip(可选)。仍会先按 IP 裁剪旧会话。
func (h *Handler) Conversations(c *gin.Context) {
	if err := h.trimAllIP(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	userIP := c.Query("user_ip")
	list, total, err := h.svc.Store.ConversationsPage(c.Request.Context(), page, pageSize, userIP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "page": page, "page_size": pageSize, "total": total})
}

// trimAllIP 收集出现过的全部 user_ip,对每个 IP 调用 TrimIPConversations(limit=20)。
func (h *Handler) trimAllIP(ctx context.Context) error {
	ips, err := h.svc.Store.ConversationIPs(ctx)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if err := h.svc.Store.TrimIPConversations(ctx, ip, 20); err != nil {
			return err
		}
	}
	return nil
}

// ConversationMessages 返回某会话全部消息。
func (h *Handler) ConversationMessages(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ok, err := h.svc.Store.ConversationExists(c.Request.Context(), id, h.clientIP(c))
	if err != nil || !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "conversation not accessible"})
		return
	}
	msgs, err := h.svc.Store.Messages(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": msgs})
}

// Commands 返回本进程已授权/已执行的 Bash 命令清单(自动放行 + 手动同意)。进程内,重启清空。
func (h *Handler) Commands(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": h.svc.Perms.Commands()})
}

// Workspaces 授权工作区列表。
func (h *Handler) Workspaces(c *gin.Context) {
	list, err := h.svc.Store.Workspaces(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": list})
}

// Stats 统计: 命中工作区 + 时间。
func (h *Handler) Stats(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")
	stats, err := h.svc.Store.StatsByWSAndTime(c.Request.Context(), from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": stats})
}

// StatsDays 按日期聚合,最新在前。
func (h *Handler) StatsDays(c *gin.Context) {
	items, err := h.svc.Store.StatsByDay(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// StatsDayIP 某日各 Client IP 成功/失败次数。
func (h *Handler) StatsDayIP(c *gin.Context) {
	date := c.Param("date")
	if date == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date required"})
		return
	}
	items, err := h.svc.Store.StatsByDayIP(c.Request.Context(), date)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"date": date, "items": items})
}

// ---- 工具授权代理 ----

// PermissionRequest 由 avatar-hook 调用:分类并(必要时)阻塞等待前端裁决。
// body: {tool_name, tool_input, conversation_id}
func (h *Handler) PermissionRequest(c *gin.Context) {
	var body struct {
		ToolName  string                 `json:"tool_name"`
		ToolInput map[string]interface{} `json:"tool_input"`
		ConvID    string                 `json:"conversation_id"`
		AgentID   string                 `json:"agent_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	convID, _ := strconv.ParseInt(body.ConvID, 10, 64)
	policyAgentID, _ := strconv.ParseInt(body.AgentID, 10, 64)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 6*time.Minute)
	defer cancel()

	dec := h.svc.HandlePermissionRequest(ctx, body.ToolName, body.ToolInput, convID, policyAgentID)
	c.JSON(http.StatusOK, gin.H{"behavior": string(dec.Behavior), "reason": dec.Reason})
}

// PermissionCompactSync 由 avatar-hook 在 Pre/PostCompact 时调用：同步清库并写系统提示。
func (h *Handler) PermissionCompactSync(c *gin.Context) {
	var body struct {
		ConvID        string `json:"conversation_id"`
		SessionID     string `json:"session_id"`
		Trigger       string `json:"trigger"`
		HookEventName string `json:"hook_event_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	convID, _ := strconv.ParseInt(body.ConvID, 10, 64)
	if convID <= 0 {
		c.JSON(http.StatusOK, gin.H{"ok": true, "skipped": true})
		return
	}
	// PostCompact / preCompact 都做幂等同步；PreCompact 也同步以免仅 Pre 触发时漏清
	if err := h.svc.SyncPlatformAfterCompact(c.Request.Context(), convID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "trigger": body.Trigger, "hook": body.HookEventName, "session_id": body.SessionID})
}

// ConversationCompress E-bot 长对话：引擎原生压缩 + 清库。
func (h *Handler) ConversationCompress(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	ok, err := h.svc.Store.ConversationExists(c.Request.Context(), id, h.clientIP(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	// E-bot 使用配置的 Claude CLI
	bin := h.svc.Cfg.ClaudeBin
	if err := h.svc.CompressConversation(c.Request.Context(), id, "claude", bin, h.svc.Cfg.WorkspaceRoot); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PermissionPending 返回挂起中的授权(前端重连补齐)。query: conversation_id?
// 管理台 JWT 可见所有挂起项(含 user_ip=admin 的数字员工/项目协作会话)；匿名仍按 ClientIP 过滤。
func (h *Handler) PermissionPending(c *gin.Context) {
	convID, _ := strconv.ParseInt(c.Query("conversation_id"), 10, 64)
	list := h.svc.Perms.Pending(convID)
	myIP := h.clientIP(c)
	adminOK := h.hasAdminJWT(c)
	out := make([]interface{}, 0, len(list))
	for _, p := range list {
		if adminOK || myIP == "" || p.ClientIP == myIP || p.ClientIP == "" {
			out = append(out, p)
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// hasAdminJWT 判断请求是否携带有效管理员 Bearer。
func (h *Handler) hasAdminJWT(c *gin.Context) bool {
	hAuth := c.GetHeader("Authorization")
	if !strings.HasPrefix(hAuth, "Bearer ") {
		return false
	}
	tok := strings.TrimSpace(strings.TrimPrefix(hAuth, "Bearer "))
	if tok == "" || h.svc.Cfg.JWTSecret == "" {
		return false
	}
	_, err := auth.ParseToken(h.svc.Cfg.JWTSecret, tok)
	return err == nil
}

// PermissionDecide 前端裁决。body: {request_id, behavior:"allow"|"deny"}
func (h *Handler) PermissionDecide(c *gin.Context) {
	var body struct {
		RequestID string `json:"request_id"`
		Behavior  string `json:"behavior"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.RequestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request_id required"})
		return
	}
	b := parseBehavior(body.Behavior)
	if err := h.svc.Perms.Decide(body.RequestID, b, "user"); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown or already resolved"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// parseBehavior 把前端字符串转成裁决枚举(仅 allow 放行,其余视为拒绝)。
func parseBehavior(s string) permission.Behavior {
	// 仅当以 'a' 开头(即 "allow")时放行;"deny"/空/非法一律拒绝。
	if len(s) > 0 && s[0] == 'a' {
		return permission.Allow
	}
	return permission.Deny
}

// PermissionAuto 开关某会话的 AI 自动审核。body: {conversation_id, enabled}
func (h *Handler) PermissionAuto(c *gin.Context) {
	var body struct {
		ConversationID int64 `json:"conversation_id"`
		Enabled        bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ConversationID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation_id required"})
		return
	}
	owner, err := h.svc.Store.ConversationOwner(c.Request.Context(), body.ConversationID)
	if err != nil || !store.CanControlAuto(owner, h.clientIP(c)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "conversation not accessible"})
		return
	}
	h.svc.Perms.SetAuto(body.ConversationID, body.Enabled)
	// 管理台数字员工:持久化到 managed_agents,刷新/重启后仍生效
	if agentID, _ := h.svc.Store.ConversationAgentID(c.Request.Context(), body.ConversationID); agentID > 0 {
		_ = h.svc.Store.SetManagedAgentAutoReview(c.Request.Context(), agentID, body.Enabled)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": body.Enabled})
}
