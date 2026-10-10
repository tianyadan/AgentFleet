package service

import (
	"context"
	"fmt"
	"strings"

	"atolla/server/internal/store"
)

// ReceptionistPublic 公开接口返回的前台助理摘要。
type ReceptionistPublic struct {
	Configured bool   `json:"configured"`
	ID         int64  `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	FolderName string `json:"folder_name,omitempty"`
	Engine     string `json:"engine,omitempty"`
}

// GetReceptionistPublic 公开拉取前台助理（无则 configured=false）。
func (s *Service) GetReceptionistPublic(ctx context.Context) (ReceptionistPublic, error) {
	a, err := s.Store.GetReceptionist(ctx)
	if err != nil {
		return ReceptionistPublic{}, err
	}
	if a == nil {
		return ReceptionistPublic{Configured: false}, nil
	}
	folder := ""
	if a.FolderID > 0 {
		if folders, e := s.Store.ListAgentFolders(ctx); e == nil {
			for _, f := range folders {
				if f.ID == a.FolderID {
					folder = f.Name
					break
				}
			}
		}
	}
	return ReceptionistPublic{
		Configured: true,
		ID:         a.ID,
		Name:       a.Name,
		AvatarURL:  a.AvatarURL,
		FolderName: folder,
		Engine:     a.Engine,
	}, nil
}

// EnsureVisitorConversation 确保访客有当前会话（没有则新建）。
func (s *Service) EnsureVisitorConversation(ctx context.Context, v *store.Visitor, agentID int64, userIP string) (int64, error) {
	if v == nil || agentID <= 0 {
		return 0, fmt.Errorf("visitor/agent required")
	}
	if v.CurrentConversationID > 0 {
		owner, _ := s.Store.ConversationOwner(ctx, v.CurrentConversationID)
		hasV, _ := s.Store.ConversationHasVisitor(ctx, v.CurrentConversationID)
		if hasV {
			_ = owner
			return v.CurrentConversationID, nil
		}
	}
	convID, err := s.Store.CreateVisitorConversation(ctx, v.ID, agentID, userIP)
	if err != nil {
		return 0, err
	}
	if err := s.Store.SetVisitorCurrentConversation(ctx, v.ID, convID); err != nil {
		return 0, err
	}
	v.CurrentConversationID = convID
	return convID, nil
}

// NewVisitorConversation 结束当前上下文：新建会话行并切换 current（保留旧行）。
func (s *Service) NewVisitorConversation(ctx context.Context, v *store.Visitor, agentID int64, userIP string) (int64, error) {
	if v == nil || agentID <= 0 {
		return 0, fmt.Errorf("visitor/agent required")
	}
	convID, err := s.Store.CreateVisitorConversation(ctx, v.ID, agentID, userIP)
	if err != nil {
		return 0, err
	}
	if err := s.Store.SetVisitorCurrentConversation(ctx, v.ID, convID); err != nil {
		return 0, err
	}
	v.CurrentConversationID = convID
	return convID, nil
}

// AskReceptionist 前台访客提问（隔离会话，不写数字人主会话）。
func (s *Service) AskReceptionist(ctx context.Context, visitorID, agentID, convID int64, question, ip, ua string, onEvent AskEventSink) (*store.Message, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("empty question")
	}
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		return nil, fmt.Errorf("agent not found")
	}
	occ, _ := s.Store.GetOccupancy(ctx, agentID)
	if OccupancyBlocksUserChat(occ) {
		return nil, &OccupancyBusyError{Occ: occ}
	}
	_ = s.Store.ClearOccupancy(ctx, agentID)
	if err := s.TryAcquireOccupancy(ctx, store.Occupancy{
		AgentID: agentID, SourceType: SourceDirect, SourceID: fmt.Sprintf("visitor:%d", visitorID),
		SourceName: "前台访客", TaskName: truncate(question, 120),
	}); err != nil {
		return nil, err
	}
	defer s.ReleaseOccupancy(context.WithoutCancel(ctx), agentID)

	_ = s.Store.TouchVisitor(ctx, visitorID)
	// 审计在 askManagedCore：用户消息 INSERT 后 Begin，assistant 落库后 Finish。
	return s.askManagedCore(ctx, a, convID, question, askCoreOpts{
		bindMainConv: false,
		visitorAudit: &visitorAskAudit{VisitorID: visitorID, IP: ip, UA: ua},
	}, onEvent)
}

// EnrichAgentsFolderNames 为列表补充分组名。
func (s *Service) EnrichAgentsFolderNames(ctx context.Context, items []store.ManagedAgent) []store.ManagedAgent {
	folders, err := s.Store.ListAgentFolders(ctx)
	if err != nil || len(folders) == 0 {
		return items
	}
	m := map[int64]string{}
	for _, f := range folders {
		m[f.ID] = f.Name
	}
	for i := range items {
		if items[i].FolderID > 0 {
			items[i].FolderName = m[items[i].FolderID]
		}
	}
	return items
}
