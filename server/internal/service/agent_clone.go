package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"colleague-avatar/server/internal/store"
)

// AgentStatusInitializing 复制后隐藏初始化尚未完成。
const AgentStatusInitializing = "initializing"

// ErrAgentInitializing 初始化中不可普通 Ask / 压缩 / 协作占用。
var ErrAgentInitializing = errors.New("员工初始化中")

// CloneSummaryMaxRunes 源员工总结字数上限。
const CloneSummaryMaxRunes = 1200

// DefaultCloneSummaryText 源无记忆时的占位总结。
const DefaultCloneSummaryText = "无历史可继承，仅使用系统提示词。"

var (
	cloneWaitPoll    = 2 * time.Second
	cloneWaitTimeout = 10 * time.Minute
)

type cloneJob struct {
	cancel context.CancelFunc
}

func (s *Service) cloneJobs() *sync.Map {
	if s.cloneJobMap == nil {
		s.cloneJobMap = &sync.Map{}
	}
	return s.cloneJobMap
}

// GuardAgentUsable 初始化中的员工不可对外执行任务。
func GuardAgentUsable(a *store.ManagedAgent) error {
	if a != nil && a.Status == AgentStatusInitializing {
		return ErrAgentInitializing
	}
	return nil
}

// IsInitializingConflict 是否应返回 409。
func IsInitializingConflict(err error) bool {
	return errors.Is(err, ErrAgentInitializing)
}

// SourceReadyForSummary 源员工空闲且无占用时才可 SilentAsk 总结。
func SourceReadyForSummary(status string, occ *store.Occupancy) bool {
	if occ != nil {
		return false
	}
	switch status {
	case "running", "waiting", "compressing", AgentStatusInitializing:
		return false
	default:
		return true
	}
}

// SourceHasEngineMemory 是否有可 resume 的引擎记忆或平台用户消息。
func SourceHasEngineMemory(sessionID string, userMsgCount int) bool {
	if strings.TrimSpace(sessionID) != "" {
		return true
	}
	return userMsgCount > 0
}

// DefaultCloneName 复制弹窗默认名称。
func DefaultCloneName(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		src = "未命名员工"
	}
	return src + " 副本"
}

// ChooseCloneSummary 无记忆用占位文案，有记忆则截断引擎输出。
func ChooseCloneSummary(hasMemory bool, asked string) string {
	if !hasMemory {
		return DefaultCloneSummaryText
	}
	asked = strings.TrimSpace(asked)
	if asked == "" {
		return DefaultCloneSummaryText
	}
	return TruncateRunes(asked, CloneSummaryMaxRunes)
}

// BuildCloneSummaryPrompt 请源员工整理可继承背景。
func BuildCloneSummaryPrompt(srcName, workspace string) string {
	var b strings.Builder
	b.WriteString("【系统内部整理，不要当作对用户的回复】你是数字员工「")
	b.WriteString(srcName)
	b.WriteString("」。请用条目总结你掌握的重要知识、经验、项目背景、仓库约定与未完成事项。")
	b.WriteString("控制在 1200 字以内；不要问候、不要写工具过程。")
	if strings.TrimSpace(workspace) != "" {
		b.WriteString("若工作区存在 projects/*/summary.md，可 Read 这些文件后再总结。")
	}
	return b.String()
}

// BuildCloneInitPrompt 新员工隐藏初始化用户消息（不落库）。
func BuildCloneInitPrompt(newName, srcName, summary string) string {
	return "【隐藏初始化】你是新员工「" + newName + "」，由同事「" + srcName +
		"」复制配置而来。请内化下列背景，之后用自己的身份工作；不要复述本段，不要主动说自己是复制来的。\n---\n" + summary
}

// NewCloneFromSource 从源员工生成插入草稿（不定时任务、不带会话）。
func NewCloneFromSource(src store.ManagedAgent, name, avatar string) store.ManagedAgent {
	av := strings.TrimSpace(avatar)
	if av == "" {
		av = strings.TrimSpace(src.AvatarURL)
	}
	return store.ManagedAgent{
		Name:            strings.TrimSpace(name),
		AvatarURL:       av,
		FolderID:        src.FolderID,
		Engine:          src.Engine,
		BinPath:         src.BinPath,
		RulesPrompt:     src.RulesPrompt,
		AutoReview:      src.AutoReview,
		TaskPlanEnabled: src.TaskPlanEnabled,
		AllowWrite:      src.AllowWrite,
		AllowNetwork:    src.AllowNetwork,
		AllowRm:         src.AllowRm,
		AllowBrowser:    src.AllowBrowser,
		WorkspacePath:   src.WorkspacePath,
		Status:          AgentStatusInitializing,
		ClonedFromID:    src.ID,
	}
}

func shouldEmitTaskPlanAsk(a *store.ManagedAgent, skip bool) bool {
	if skip {
		return false
	}
	return shouldEmitTaskPlan(a)
}

// CopyManagedAgent 复制配置、新建独立会话，并异步隐藏初始化。
func (s *Service) CopyManagedAgent(ctx context.Context, srcID int64, name, avatarURL string) (*store.ManagedAgent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	src, err := s.Store.GetManagedAgent(ctx, srcID)
	if err != nil || src == nil {
		if src == nil && err == nil {
			return nil, fmt.Errorf("agent not found")
		}
		return nil, err
	}
	if src.Status == AgentStatusInitializing {
		return nil, ErrAgentInitializing
	}
	draft := NewCloneFromSource(*src, name, avatarURL)
	id, err := s.Store.InsertClonedManagedAgent(ctx, draft)
	if err != nil {
		return nil, err
	}
	convID, err := s.Store.CreateAgentConversation(ctx, id)
	if err != nil {
		_ = s.Store.DeleteManagedAgent(ctx, id)
		return nil, err
	}
	_ = s.Store.SetManagedAgentConversation(ctx, id, convID)
	clone, err := s.Store.GetManagedAgent(ctx, id)
	if err != nil || clone == nil {
		return nil, fmt.Errorf("clone load failed")
	}
	s.startCloneInit(clone.ID, src.ID)
	return clone, nil
}

// RetryCloneInit 初始化失败后重跑总结与注入。
func (s *Service) RetryCloneInit(ctx context.Context, cloneID int64) (*store.ManagedAgent, error) {
	a, err := s.Store.GetManagedAgent(ctx, cloneID)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return nil, fmt.Errorf("agent not found")
		}
		return nil, err
	}
	if a.ClonedFromID <= 0 {
		return nil, fmt.Errorf("not a cloned agent")
	}
	if a.Status == AgentStatusInitializing {
		return nil, ErrAgentInitializing
	}
	_ = s.Store.SetManagedAgentStatus(ctx, cloneID, AgentStatusInitializing, "", 0, a.ConversationID)
	s.startCloneInit(cloneID, a.ClonedFromID)
	return s.Store.GetManagedAgent(ctx, cloneID)
}

// DeleteManaged 解聘：取消初始化任务后删库。
func (s *Service) DeleteManaged(ctx context.Context, id int64) error {
	s.cancelCloneJob(id)
	s.StopManaged(id)
	return s.Store.DeleteManagedAgent(ctx, id)
}

func (s *Service) startCloneInit(cloneID, srcID int64) {
	s.cancelCloneJob(cloneID)
	ctx, cancel := context.WithCancel(context.Background())
	s.cloneJobs().Store(cloneID, cloneJob{cancel: cancel})
	go func() {
		defer s.cloneJobs().Delete(cloneID)
		s.runCloneInit(ctx, cloneID, srcID)
	}()
}

func (s *Service) cancelCloneJob(id int64) {
	if v, ok := s.cloneJobs().Load(id); ok {
		if j, ok := v.(cloneJob); ok && j.cancel != nil {
			j.cancel()
		}
		s.cloneJobs().Delete(id)
	}
}

func (s *Service) runCloneInit(ctx context.Context, cloneID, srcID int64) {
	fail := func(msg string) {
		_ = s.Store.SetManagedAgentStatus(context.Background(), cloneID, "error", msg, 0, 0)
	}
	deadline := time.Now().Add(cloneWaitTimeout)
	for {
		if ctx.Err() != nil {
			return
		}
		src, err := s.Store.GetManagedAgent(ctx, srcID)
		if err != nil || src == nil {
			fail("源员工不存在，无法继承记忆")
			return
		}
		occ, _ := s.Store.GetOccupancy(ctx, srcID)
		if SourceReadyForSummary(src.Status, occ) {
			break
		}
		if time.Now().After(deadline) {
			fail("等待源员工空闲超时")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(cloneWaitPoll):
		}
	}
	if ctx.Err() != nil {
		return
	}
	clone, err := s.Store.GetManagedAgent(ctx, cloneID)
	if err != nil || clone == nil {
		return
	}
	src, _ := s.Store.GetManagedAgent(ctx, srcID)
	if src == nil {
		fail("源员工不存在，无法继承记忆")
		return
	}
	summary := DefaultCloneSummaryText
	hasMem := false
	if src.ConversationID > 0 {
		meta, _ := s.Store.GetConversationEngineMeta(ctx, src.ConversationID)
		n, _ := s.Store.CountUserMessages(ctx, src.ConversationID)
		hasMem = SourceHasEngineMemory(meta.SessionID, n)
	}
	if hasMem {
		if err := s.TryAcquireOccupancy(ctx, store.Occupancy{
			AgentID: src.ID, SourceType: SourceCloneInit, SourceID: fmt.Sprintf("clone-init-%d", cloneID),
			SourceName: src.Name, TaskName: "为副本整理记忆",
		}); err != nil {
			fail("源员工忙碌，无法整理记忆")
			return
		}
		msg, askErr := s.silentAsk(ctx, src, BuildCloneSummaryPrompt(src.Name, src.WorkspacePath), true)
		s.ReleaseOccupancy(context.WithoutCancel(ctx), src.ID)
		if askErr != nil {
			fail("源员工总结失败: " + askErr.Error())
			return
		}
		text := ""
		if msg != nil {
			text = msg.Content
		}
		summary = ChooseCloneSummary(true, text)
	} else {
		summary = ChooseCloneSummary(false, "")
	}
	initQ := BuildCloneInitPrompt(clone.Name, src.Name, summary)
	clone2, _ := s.Store.GetManagedAgent(ctx, cloneID)
	if clone2 == nil {
		return
	}
	_, askErr := s.silentAsk(ctx, clone2, initQ, true)
	if ctx.Err() != nil {
		return
	}
	if askErr != nil {
		fail("隐藏初始化失败: " + askErr.Error())
		return
	}
	_ = s.Store.SetManagedAgentStatus(context.WithoutCancel(ctx), cloneID, "idle", "", 0, clone.ConversationID)
}
