package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"colleague-avatar/server/internal/agent"
	"colleague-avatar/server/internal/store"
)

// AgentStatusCompressing 数字员工「压缩记忆」工作状态。
const AgentStatusCompressing = "compressing"

// CompactNoticeText 平台侧压缩后保留的唯一提示文案。
const CompactNoticeText = "📦 上下文已由引擎压缩，会话已保留"

// CompactStatusAfterSync 自动压缩同步结束后应恢复的状态。
// prev 为进入压缩前的状态；若已是 compressing（手动已抢先置位），视为独立压缩任务 → idle。
func CompactStatusAfterSync(prev string) string {
	switch prev {
	case "running":
		return "running"
	default:
		return "idle"
	}
}

// ReinjectionPrefix 压缩后下一轮系统提示强化段。
func ReinjectionPrefix() string {
	return "【压缩后续聊】引擎上下文刚完成压缩。请重新严格遵循下列数字人员设、权限与工作区策略。"
}

// ApplySystemReinject 若需要 reinject，在系统提示前加强化段。
func ApplySystemReinject(sys string, need bool) string {
	if !need {
		return sys
	}
	p := ReinjectionPrefix()
	sys = strings.TrimSpace(sys)
	if sys == "" {
		return p
	}
	return p + "\n\n" + sys
}

// consumeSystemReinject 读取并清除 reinject 标记，返回可能加强后的系统提示。
func (s *Service) consumeSystemReinject(ctx context.Context, convID int64, sys string) string {
	meta, err := s.Store.GetConversationEngineMeta(ctx, convID)
	if err != nil || !meta.NeedsSystemReinject {
		return sys
	}
	_ = s.Store.SetConversationNeedsSystemReinject(ctx, convID, false)
	return ApplySystemReinject(sys, true)
}

// IsCompactHookEvent 判断是否为引擎压缩生命周期 hook。
func IsCompactHookEvent(name string) bool {
	switch strings.TrimSpace(name) {
	case "PreCompact", "PostCompact", "preCompact":
		return true
	default:
		return false
	}
}

// LatestTurnToPreserve 提取清库后应保留的本轮 user / assistant（跳过 command 等）。
func LatestTurnToPreserve(msgs []store.Message) (userContent, assistantContent string) {
	lastUser := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && strings.TrimSpace(msgs[i].Content) != "" {
			lastUser = i
			userContent = msgs[i].Content
			break
		}
	}
	if lastUser < 0 {
		return "", ""
	}
	for i := len(msgs) - 1; i > lastUser; i-- {
		if msgs[i].Role == "assistant" && strings.TrimSpace(msgs[i].Content) != "" {
			// 跳过压缩提示本身
			if strings.TrimSpace(msgs[i].Content) == CompactNoticeText {
				continue
			}
			assistantContent = msgs[i].Content
			break
		}
	}
	return userContent, assistantContent
}

// SyncPlatformAfterCompact 自动压缩同步（Hook / 延迟 JSONL）：始终保留本轮 user+assistant。
func (s *Service) SyncPlatformAfterCompact(ctx context.Context, convID int64) error {
	return s.syncAfterCompact(ctx, convID, true)
}

// SyncPlatformAfterCompactFull 手动压缩：全量清空，仅留系统提示。
func (s *Service) SyncPlatformAfterCompactFull(ctx context.Context, convID int64) error {
	return s.syncAfterCompact(ctx, convID, false)
}

func (s *Service) markCompactPending(convID int64) {
	if convID <= 0 {
		return
	}
	if s.compactPending == nil {
		return
	}
	s.compactPending.Store(convID, true)
}

func (s *Service) consumeCompactPending(convID int64) bool {
	if convID <= 0 || s.compactPending == nil {
		return false
	}
	_, ok := s.compactPending.LoadAndDelete(convID)
	return ok
}

// syncAfterCompact 清库并写系统提示；keepLatest 时保留本轮对话，避免回复落库后被 Hook 二次清空。
func (s *Service) syncAfterCompact(ctx context.Context, convID int64, keepLatest bool) error {
	if convID <= 0 {
		return fmt.Errorf("invalid conversation")
	}
	agentID, prevStatus, _ := s.Store.ManagedAgentStatusByConversation(ctx, convID)
	started := time.Now()
	preserveUser, preserveAsst := "", ""
	if keepLatest {
		if msgs, err := s.Store.Messages(ctx, convID); err == nil {
			preserveUser, preserveAsst = LatestTurnToPreserve(msgs)
		}
	}
	// 自动同步不把员工打成「压缩记忆」，避免与手动压缩状态竞态导致二次全量清空
	if !keepLatest && agentID > 0 && prevStatus != AgentStatusCompressing {
		_ = s.Store.SetManagedAgentStatus(ctx, agentID, AgentStatusCompressing, "", 0, convID)
	} else if keepLatest && agentID > 0 && prevStatus == "running" {
		// 回合中自动压缩：短暂标记，结束后恢复 running
		_ = s.Store.SetManagedAgentStatus(ctx, agentID, AgentStatusCompressing, "", 0, convID)
	}

	if err := s.Store.ClearConversationMessages(ctx, convID); err != nil {
		s.finishCompactAgentStatus(ctx, agentID, prevStatus, convID, started, err)
		return err
	}
	_, err := s.Store.InsertMessage(ctx, &store.Message{
		ConversationID:     convID,
		Role:               "assistant",
		Content:            CompactNoticeText,
		Status:             "ok",
		ExcludeFromContext: true,
	})
	if err != nil {
		s.finishCompactAgentStatus(ctx, agentID, prevStatus, convID, started, err)
		return err
	}
	if preserveUser != "" {
		_, _ = s.Store.InsertMessage(ctx, &store.Message{
			ConversationID: convID, Role: "user", Content: preserveUser, Status: "ok",
		})
	}
	if preserveAsst != "" {
		_, _ = s.Store.InsertMessage(ctx, &store.Message{
			ConversationID: convID, Role: "assistant", Content: preserveAsst, Status: "ok",
		})
	}
	if err := s.Store.SetConversationNeedsSystemReinject(ctx, convID, true); err != nil {
		s.finishCompactAgentStatus(ctx, agentID, prevStatus, convID, started, err)
		return err
	}
	s.finishCompactAgentStatus(ctx, agentID, prevStatus, convID, started, nil)
	return nil
}

// finishCompactAgentStatus 压缩结束后恢复/收尾数字员工状态并写入耗时。
func (s *Service) finishCompactAgentStatus(ctx context.Context, agentID int64, prevStatus string, convID int64, started time.Time, syncErr error) {
	if agentID <= 0 {
		return
	}
	ms := int(time.Since(started).Milliseconds())
	if a, _ := s.Store.GetManagedAgent(ctx, agentID); a != nil && a.RunStartedAt != nil {
		if d := int(time.Since(*a.RunStartedAt).Milliseconds()); d > ms {
			ms = d
		}
	}
	if ms < 1 {
		ms = 1
	}
	if syncErr != nil {
		_ = s.Store.SetManagedAgentStatus(ctx, agentID, "error", syncErr.Error(), ms, convID)
		return
	}
	next := CompactStatusAfterSync(prevStatus)
	if next == "running" {
		_ = s.Store.SetManagedAgentStatus(ctx, agentID, "running", "", 0, convID)
		return
	}
	_ = s.Store.SetManagedAgentStatus(ctx, agentID, "idle", "", ms, convID)
}

// CompressConversation 对指定会话执行引擎原生压缩并同步平台历史。
func (s *Service) CompressConversation(ctx context.Context, convID int64, engine, bin, dir string) error {
	if convID <= 0 {
		return fmt.Errorf("invalid conversation")
	}
	meta, err := s.Store.GetConversationEngineMeta(ctx, convID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(meta.SessionID) != "" {
		if err := agent.RunEngineCompact(ctx, bin, engine, dir, meta.SessionID, s.Cfg.AskTimeout()); err != nil {
			return err
		}
	}
	return s.SyncPlatformAfterCompactFull(ctx, convID)
}

// CompressManagedChat 数字员工：原生压缩 + 清库（保留 engine session），并展示「压缩记忆」状态。
func (s *Service) CompressManagedChat(ctx context.Context, agentID int64) error {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return fmt.Errorf("agent not found")
		}
		return err
	}
	if a.ConversationID == 0 {
		return nil
	}
	if err := GuardAgentUsable(a); err != nil {
		return err
	}
	if occ, _ := s.Store.GetOccupancy(ctx, agentID); OccupancyBlocksUserChat(occ) {
		return &OccupancyBusyError{Occ: occ}
	}
	dir := s.Cfg.WorkspaceRoot
	if ws := strings.TrimSpace(a.WorkspacePath); ws != "" {
		dir = ws
	}
	bin := a.BinPath
	prev := a.Status
	start := time.Now()
	_ = s.Store.SetManagedAgentStatus(ctx, agentID, AgentStatusCompressing, "", 0, a.ConversationID)
	err = s.CompressConversation(ctx, a.ConversationID, a.Engine, bin, dir)
	if err != nil {
		ms := int(time.Since(start).Milliseconds())
		if ms < 1 {
			ms = 1
		}
		cur, _ := s.Store.GetManagedAgent(ctx, agentID)
		if cur != nil && cur.Status == AgentStatusCompressing {
			_ = s.Store.SetManagedAgentStatus(ctx, agentID, "error", err.Error(), ms, a.ConversationID)
		}
		return err
	}
	cur, _ := s.Store.GetManagedAgent(ctx, agentID)
	if cur != nil && cur.Status == AgentStatusCompressing {
		s.finishCompactAgentStatus(ctx, agentID, prev, a.ConversationID, start, nil)
	}
	return nil
}

// EnsureManagedConversation 若尚无会话则创建并绑定（不换掉已有会话；供自动审核等内部使用）。
func (s *Service) EnsureManagedConversation(ctx context.Context, agentID int64) (int64, error) {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return 0, fmt.Errorf("agent not found")
		}
		return 0, err
	}
	if a.ConversationID > 0 {
		return a.ConversationID, nil
	}
	convID, err := s.Store.CreateAgentConversation(ctx, agentID)
	if err != nil {
		return 0, err
	}
	if err := s.Store.SetManagedAgentConversation(ctx, agentID, convID); err != nil {
		return 0, err
	}
	if a.AutoReview {
		s.Perms.SetAuto(convID, true)
	}
	return convID, nil
}
