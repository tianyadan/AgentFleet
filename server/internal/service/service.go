package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"colleague-avatar/server/config"
	"colleague-avatar/server/internal/agent"
	"colleague-avatar/server/internal/auth"
	"colleague-avatar/server/internal/dbquery"
	"colleague-avatar/server/internal/notify"
	"colleague-avatar/server/internal/permission"
	"colleague-avatar/server/internal/store"
	"colleague-avatar/server/internal/testsrv"
)

const (
	ModeSingle = "single"
	ModeChat   = "chat"
)

// QuestionReq 提问请求。
type QuestionReq struct {
	Question       string `json:"question"`
	Workspace      string `json:"workspace"`       // 可选: 指定授权工作区路径
	ConversationID int64  `json:"conversation_id"` // 会话id;0=新建
	Mode           string `json:"mode"`            // single / chat
	UserIP         string `json:"-"`               // 由 handler 注入
}

// Service 顶层业务编排。
type Service struct {
	Cfg      config.Config
	Store    *store.Store
	Agent    *agent.Runner
	DBQuery  *dbquery.Client
	Perms    *permission.Hub
	Reviewer *permission.Reviewer
	TestHub  *testsrv.Hub
	Notify   *notify.Notifier
	Sched    *Scheduler
	Memory   MemoryService
	WF       *WorkflowEngine

	runMu sync.Mutex
	runs  map[int64]*managedRun // managed agent 进行中的 Ask

	cloneJobMap     *sync.Map
	compactPending  *sync.Map // convID -> true，Codex JSONL 压缩延后到回复落库后再同步
}

type managedRun struct {
	cancel context.CancelFunc
}

func New(cfg config.Config, st *store.Store) *Service {
	svc := &Service{
		Cfg:      cfg,
		Store:    st,
		Agent:    agent.NewRunner(cfg.ClaudeBin),
		DBQuery:  mustDBQuery(cfg.DataDBDSN),
		Perms:    permission.NewHub(cfg.PermissionWaitSeconds()),
		Reviewer: newReviewer(cfg),
		TestHub:  testsrv.NewHub(cfg.PermissionWaitSeconds()),
		Notify:   notify.New(cfg.NotifyScript, cfg.BarkNotify),
		runs:           map[int64]*managedRun{},
		cloneJobMap:    &sync.Map{},
		compactPending: &sync.Map{},
	}
	svc.Sched = NewScheduler(svc)
	svc.Memory = NewMemoryService(st)
	svc.WF = newWorkflowEngine(svc)
	svc.Sched.Reload(context.Background())
	// 重启后未完成工作流进入 waiting_recovery，再尝试自动自检续跑（3 分钟窗口）
	if n, err := st.MarkIncompleteRunsWaitingRecovery(context.Background()); err != nil {
		log.Printf("mark waiting_recovery runs: %v", err)
	} else if n > 0 {
		log.Printf("marked %d incomplete workflow runs as waiting_recovery", n)
	}
	if n, err := st.InterruptIncompleteNodeExecutions(context.Background()); err != nil {
		log.Printf("interrupt incomplete node executions: %v", err)
	} else if n > 0 {
		log.Printf("interrupted %d incomplete node executions", n)
	}
	_ = st.ClearAllOccupancy(context.Background())
	svc.BeginWorkflowRecoveryWindow()
	return svc
}

// HandlePermissionRequest 由 hook 调用:分类工具调用,只读自动放行、灾难自动拒绝、
// 其余登记为待决请求并阻塞等待前端裁决(超时/断连按拒绝)。
// policyAgentID>0 时按该数字员工策略裁决(委托场景:会话挂调用方,策略用被调方)。
func (s *Service) HandlePermissionRequest(ctx context.Context, tool string, input map[string]interface{}, convID, policyAgentID int64) permission.Decision {
	if !s.Cfg.PermissionEnabled {
		return permission.Decision{Behavior: permission.Deny, Reason: "未启用授权代理"}
	}
	codeWS, _ := s.Store.CodeWorkspaces(ctx)
	roots := make([]string, 0, len(codeWS))
	for _, w := range codeWS {
		roots = append(roots, w.Path)
	}
	var agentPolicy *permission.AgentPolicy
	var policyAgentName string
	lookupID := policyAgentID
	if lookupID <= 0 {
		lookupID, _ = s.Store.ConversationAgentID(ctx, convID)
	}
	if lookupID > 0 {
		if ag, _ := s.Store.GetManagedAgent(ctx, lookupID); ag != nil {
			p := permission.AgentPolicy{
				AllowWrite: ag.AllowWrite, AllowNetwork: ag.AllowNetwork, AllowRm: ag.AllowRm,
				WorkspacePath: ag.WorkspacePath,
			}
			agentPolicy = &p
			policyAgentName = ag.Name
			if ws := strings.TrimSpace(ag.WorkspacePath); ws != "" {
				roots = []string{ws}
			}
		}
	}
	dec := permission.Classify(tool, input, roots)
	if agentPolicy != nil {
		dec = permission.ApplyAgentPolicy(tool, input, dec, *agentPolicy)
	}
	if dec.Behavior != permission.Ask {
		if tool == "Bash" || tool == "WebFetch" || tool == "WebSearch" {
			decision := "allow"
			if dec.Behavior == permission.Deny {
				decision = "deny"
			}
			cmdText := str(input, "command")
			if cmdText == "" {
				cmdText = summarize(tool, input)
			}
			s.recordCommand(ctx, convID, tool, cmdText, decision, "system", "", "", dec.Reason)
		}
		return dec
	}
	// 编排级「允许执行所有命令」：除 rm 外直接放行，不弹窗
	if s.Perms.IsAllowAllExceptRm(convID) && !permission.LooksLikeRm(tool, input) {
		summary := summarize(tool, input)
		owner, _ := s.Store.ConversationOwner(ctx, convID)
		s.Perms.AutoApprove(tool, summary, "编排允许全部命令", convID, owner)
		s.recordCommand(ctx, convID, tool, summary, "allow", "system", "", "", "编排允许全部命令(除rm)")
		return permission.Decision{Behavior: permission.Allow, Reason: "编排允许全部命令"}
	}
	owner, _ := s.Store.ConversationOwner(ctx, convID)
	summary := summarize(tool, input)

	note := ""
	if policyAgentID > 0 && policyAgentName != "" {
		note = "同事「" + policyAgentName + "」申请授权"
	}
	if s.Reviewer != nil && s.Perms.IsAuto(convID) {
		allow, reason := s.review(ctx, convID, tool, summary, roots)
		if allow {
			s.Perms.AutoApprove(tool, summary, reason, convID, owner)
			s.recordCommand(ctx, convID, tool, summary, "allow", "ai", "", "", reason)
			return permission.Decision{Behavior: permission.Allow, Reason: "AI 审核放行:" + reason}
		}
		aiNote := "AI 建议拒绝:" + reason
		if reason == "" {
			aiNote = "AI 审核不可用,请人工确认"
		}
		if note != "" {
			note += " · " + aiNote
		} else {
			note = aiNote
		}
	}

	meaning, risk := s.explain(ctx, tool, summary, roots)

	req := s.Perms.Register(tool, input, summary, note, convID, owner)
	req.Meaning = meaning
	req.Risk = risk

	out := s.Perms.Wait(ctx, req.ID)
	by := "user"
	decision := "allow"
	if out.Behavior != permission.Allow {
		decision = "deny"
		if strings.Contains(out.Reason, "超时") {
			by = "timeout"
		} else if strings.Contains(out.Reason, "会话已结束") {
			by = "disconnect"
		}
	}
	s.recordCommand(ctx, convID, tool, summary, decision, by, risk, meaning, note)
	return out
}

// recordCommand 写入审计表 + 对话消息 + SSE 广播(允许与拒绝均记录)。
func (s *Service) recordCommand(ctx context.Context, convID int64, tool, cmd, decision, by, risk, meaning, note string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" && tool != "Bash" {
		cmd = tool
	}
	if cmd == "" {
		return
	}
	agentID, _ := s.Store.ConversationAgentID(ctx, convID)
	_, _ = s.Store.InsertCommandAudit(ctx, &store.CommandAudit{
		ConversationID: convID,
		AgentID:        agentID,
		ToolName:       tool,
		CommandText:    cmd,
		Decision:       decision,
		DecidedBy:      by,
		Risk:           risk,
		Meaning:        meaning,
		Note:           note,
	})
	status := "ok"
	if decision != "allow" {
		status = "denied"
	}
	payload, _ := json.Marshal(map[string]string{
		"command": cmd, "decision": decision, "by": by, "risk": risk, "meaning": meaning, "note": note, "tool": tool,
	})
	if convID > 0 {
		_, _ = s.Store.InsertMessage(ctx, &store.Message{
			ConversationID: convID,
			Role:           "command",
			Content:        string(payload),
			Status:         status,
		})
	}
	if tool == "Bash" && decision == "allow" {
		s.Perms.AppendCommand(cmd)
	}
	s.Perms.BroadcastCommand(tool, cmd, decision, by, risk, meaning, convID)
}

// persistCommandMessage 兼容旧调用(已由 recordCommand 替代)。
func (s *Service) persistCommandMessage(ctx context.Context, convID int64, cmd string) {
	s.recordCommand(ctx, convID, "Bash", cmd, "allow", "system", "", "", "")
}

// review 走缓存,未命中才真正调用审核器。返回 (是否放行, 理由)。
func (s *Service) review(ctx context.Context, convID int64, tool, summary string, roots []string) (bool, string) {
	v := s.reviewVerdict(ctx, convID, tool, summary, roots)
	if v.Err != nil {
		log.Printf("AI 审核失败 conv=%d tool=%s: %v", convID, tool, v.Err)
		return false, ""
	}
	return v.Allow, v.Reason
}

// reviewVerdict 调用审核器并返回完整结论(含含义/风险)。结果写入审核缓存。
func (s *Service) reviewVerdict(ctx context.Context, convID int64, tool, summary string, roots []string) permission.Verdict {
	key := permission.ReviewKey(convID, tool, map[string]interface{}{"s": summary})
	if allow, reason, ok := s.Perms.CachedVerdict(key); ok {
		return permission.Verdict{Allow: allow, Reason: reason}
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(s.Cfg.ReviewTimeout())*time.Second)
	defer cancel()
	v := s.Reviewer.Review(rctx, permission.ReviewRequest{ToolName: tool, Summary: summary, Roots: roots})
	if v.Err == nil {
		s.Perms.PutCached(key, v.Allow, v.Reason)
	}
	return v
}

// explain 让另一个 AI(审核器同款)解释命令含义并评估风险,随权限弹窗展示。
// 命中审核缓存时直接复用其中已解析的含义/风险,避免重复调用。
func (s *Service) explain(ctx context.Context, tool, summary string, roots []string) (meaning, risk string) {
	if s.Reviewer == nil {
		return "", ""
	}
	// 弹窗命令通常非只读;审核器会给出含义+风险。这里取缓存优先,再回退新调用。
	v := s.reviewVerdict(ctx, 0, tool, summary, roots)
	if v.Err != nil {
		return "", ""
	}
	return v.Meaning, v.Risk
}

// summarize 生成给用户看的工具摘要(截断 + 去敏感)。
func summarize(tool string, input map[string]interface{}) string {
	switch tool {
	case "Bash":
		return clip(str(input, "command"))
	case "Read", "Grep", "Glob", "LS":
		if p := str(input, "file_path"); p != "" {
			return clip(p)
		}
		if p := str(input, "path"); p != "" {
			return clip(p)
		}
		return tool
	default:
		if p := str(input, "file_path"); p != "" {
			return clip(p)
		}
	}
	return tool
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 300 {
		return string(r[:300]) + "…"
	}
	return s
}

func str(input map[string]interface{}, key string) string {
	if v, ok := input[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// newReviewer 构造 AI 权限审核器。环境沿用分身那套清洗逻辑,
// 避免审核子进程继承父会话变量。
func newReviewer(cfg config.Config) *permission.Reviewer {
	if !cfg.PermissionEnabled {
		return nil
	}
	rv := permission.NewReviewer(cfg.ClaudeBin, cfg.ReviewTimeoutSec)
	rv.Env = agent.CleanEnv(os.Environ())
	return rv
}

func mustDBQuery(dsn string) *dbquery.Client {
	c, err := dbquery.New(dsn)
	if err != nil {
		// 若未配置数据查询,置 nil(数据查询不可用但不阻塞主流程)
		return nil
	}
	return c
}

// Ask 处理一次提问(单次或长对话),SSE 流式 onChunk 回调。
// 返回最终保存的 assistant Message。
func (s *Service) Ask(ctx context.Context, req QuestionReq, onChunk func(string)) (*store.Message, error) {
	mode := req.Mode
	if mode == "" {
		mode = ModeSingle
	}
	if mode != ModeSingle && mode != ModeChat {
		return nil, fmt.Errorf("invalid mode: %s", mode)
	}

	// 确定会话 id: 有则校验归属;无则新建
	convID := req.ConversationID
	if convID == 0 {
		id, err := s.Store.CreateConversation(ctx, req.UserIP, mode)
		if err != nil {
			return nil, err
		}
		convID = id
	} else {
		ok, err := s.Store.ConversationExists(ctx, convID, req.UserIP)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("conversation not found or not owned")
		}
	}

	workspaces, err := s.Store.Workspaces(ctx)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(workspaces))
	for _, w := range workspaces {
		if w.Type == "db" {
			continue // 数据查询项目无文件系统路径,跳过授权校验
		}
		roots = append(roots, w.Path)
	}

	// 授权校验: code 工作区走文件系统授权; db 数据查询项目(path 形如 db://xxx)直接放行
	wsPath := ""
	if req.Workspace != "" {
		var matched string
		var ok bool
		if isDataWorkspace(req.Workspace, workspaces) {
			ok = true
			matched = req.Workspace
		} else {
			matched, ok = auth.Authorized(req.Workspace, roots)
		}
		if !ok {
			// 写一条 user + assistant(rejected) 消息
			s.storeRejected(ctx, convID, req.Question, req.Workspace)
			return &store.Message{ConversationID: convID, Role: "assistant", Status: "rejected", Content: "⚠️ 该工作区未授权,无法读取。"}, nil
		}
		wsPath = matched
	}

	// v0.2.18：不再拼平台历史；多轮靠引擎 --resume + 本轮问题
	history := ""

	// 工作目录: 仅取真实文件系统路径。db 数据查询项目(db://...)无对应目录,
	// 回退到第一个 code 工作区作为 cwd;数据检索由 Agent 通过后端只读接口完成。
	dir := wsPath
	if strings.HasPrefix(dir, "db://") {
		dir = ""
	}
	if dir == "" && len(roots) > 0 {
		dir = roots[0]
	}

	sysPrompt := s.SystemPrompt(ctx)
	sysPrompt = s.consumeSystemReinject(ctx, convID, sysPrompt)
	timeout := s.Cfg.AskTimeout()
	// Ask 结束务必清理本会话残留的挂起授权(唤醒 hook,通知前端出队)
	defer s.Perms.DropByConv(convID)
	hook := agent.HookSpec{}
	if s.Cfg.HookBin != "" {
		hook = agent.HookSpec{
			Bin:      s.Cfg.HookBin,
			Backend:  s.Cfg.BackendURL,
			ConvID:   convID,
			TimeoutS: s.Cfg.PermissionWaitSeconds() + 30,
		}
	}
	resume := ""
	if em, err := s.Store.GetConversationEngineMeta(ctx, convID); err == nil {
		resume = em.SessionID
	}
	onMeta := func(m agent.RunMeta) {
		s.RememberEngineMeta(context.WithoutCancel(ctx), 0, convID, m)
	}
	full, ms, aerr := s.Agent.AskStream(ctx, dir, sysPrompt, req.Question, history, onChunk, timeout, hook, nil, onMeta, resume)

	status := "ok"
	content := full
	aborted := ctx.Err() != nil && aerr != nil
	switch {
	case aborted:
		// 用户点了「终止」:保留已产出的部分答案
		status = "aborted"
		content = strings.TrimSpace(full)
		if content != "" {
			content += "\n\n_[已终止]_"
		} else {
			content = "_[已终止]_"
		}
	case aerr != nil:
		status = "error"
		content = full + "\n[agent error] " + aerr.Error()
	}

	// 落库必须脱离请求上下文:前端终止会取消 ctx,直接用 ctx 写库会失败并丢答案。
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	userMsg := &store.Message{ConversationID: convID, Role: "user", Content: req.Question, Status: status, WorkspacePath: wsPath}
	s.Store.InsertMessage(saveCtx, userMsg)
	asstMsg := &store.Message{ConversationID: convID, Role: "assistant", Content: content, Status: status, WorkspacePath: wsPath, ReplyMs: ms}
	asstID, err := s.Store.InsertMessage(saveCtx, asstMsg)
	if err != nil {
		log.Printf("落库失败 conv=%d: %v", convID, err)
		return asstMsg, nil
	}
	asstMsg.ID = asstID
	return asstMsg, nil
}

func isDataWorkspace(candidate string, workspaces []store.Workspace) bool {
	for _, w := range workspaces {
		if w.Type == "db" && w.Path == candidate {
			return true
		}
	}
	return false
}

// storeRejected 未授权时写入 user + assistant(rejected)。
func (s *Service) storeRejected(ctx context.Context, convID int64, q, ws string) {
	s.Store.InsertMessage(ctx, &store.Message{ConversationID: convID, Role: "user", Content: q, Status: "rejected", WorkspacePath: ws})
	s.Store.InsertMessage(ctx, &store.Message{ConversationID: convID, Role: "assistant", Status: "rejected", Content: "⚠️ 该工作区未授权,无法读取。", WorkspacePath: ws})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
