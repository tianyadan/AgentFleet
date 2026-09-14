package service

import (
	"context"
	"fmt"
	"strings"

	"colleague-avatar/server/internal/store"
)

const (
	SourceDirect   = "DIRECT"
	SourceWorkflow = "WORKFLOW"
	SourceInvoke   = "INVOKE"
	SourceSchedule = "SCHEDULE"
)

// OccupancyBusyError 主任务冲突。
type OccupancyBusyError struct {
	Occ *store.Occupancy
}

func (e *OccupancyBusyError) Error() string {
	if e == nil || e.Occ == nil {
		return "agent busy"
	}
	return fmt.Sprintf("agent busy: %s %s", e.Occ.SourceType, e.Occ.SourceName)
}

// TryAcquireOccupancy 尝试占用；已占用返回 OccupancyBusyError。
func (s *Service) TryAcquireOccupancy(ctx context.Context, o store.Occupancy) error {
	ok, err := s.Store.TryInsertOccupancy(ctx, o)
	if err != nil {
		return err
	}
	if !ok {
		cur, _ := s.Store.GetOccupancy(ctx, o.AgentID)
		return &OccupancyBusyError{Occ: cur}
	}
	return nil
}

// ReleaseOccupancy 释放。
func (s *Service) ReleaseOccupancy(ctx context.Context, agentID int64) {
	_ = s.Store.ClearOccupancy(ctx, agentID)
}

// MemoryService 长期记忆读写（实现可替换）。
type MemoryService interface {
	Write(ctx context.Context, m *store.AgentMemory) error
	Retrieve(ctx context.Context, agentID int64, query string, limit int) ([]store.AgentMemory, error)
}

type storeMemoryService struct {
	st *store.Store
}

// NewMemoryService 默认实现。
func NewMemoryService(st *store.Store) MemoryService {
	return &storeMemoryService{st: st}
}

func (m *storeMemoryService) Write(ctx context.Context, mem *store.AgentMemory) error {
	_, err := m.st.InsertAgentMemory(ctx, mem)
	return err
}

func (m *storeMemoryService) Retrieve(ctx context.Context, agentID int64, query string, limit int) ([]store.AgentMemory, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 5 {
		limit = 5
	}
	cands, err := m.st.ListAgentMemoriesRecent(ctx, agentID, 80)
	if err != nil {
		return nil, err
	}
	tokens := tokenize(query)
	type scored struct {
		m store.AgentMemory
		s int
	}
	var ranked []scored
	for _, c := range cands {
		sc := scoreMemory(c, tokens)
		if sc <= 0 && len(tokens) > 0 {
			continue
		}
		ranked = append(ranked, scored{c, sc})
	}
	// 无匹配时退回最近若干条
	if len(ranked) == 0 {
		if len(cands) > limit {
			cands = cands[:limit]
		}
		return cands, nil
	}
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].s > ranked[i].s {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]store.AgentMemory, len(ranked))
	for i := range ranked {
		out[i] = ranked[i].m
	}
	return out, nil
}

func tokenize(q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == ',' || r == '，' || r == '。' || r == '/' || r == '|'
	})
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if len([]rune(f)) < 2 {
			continue
		}
		out = append(out, f)
	}
	return out
}

func scoreMemory(m store.AgentMemory, tokens []string) int {
	if len(tokens) == 0 {
		return 1
	}
	blob := strings.ToLower(m.Title + " " + m.Summary)
	sc := 0
	for _, t := range tokens {
		if strings.Contains(blob, t) {
			sc += 2
			if strings.Contains(strings.ToLower(m.Title), t) {
				sc += 2
			}
		}
	}
	return sc
}

// FormatMemoriesForPrompt 注入块。
func FormatMemoriesForPrompt(mems []store.AgentMemory) string {
	if len(mems) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<团队长期记忆摘要>\n")
	for i, m := range mems {
		b.WriteString(fmt.Sprintf("%d. %s\n%s\n", i+1, m.Title, truncate(m.Summary, 400)))
	}
	b.WriteString("</团队长期记忆摘要>\n")
	return b.String()
}
