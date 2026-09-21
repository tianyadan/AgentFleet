package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"colleague-avatar/server/internal/store"
)

// CreateWorkflow 新建编排。
func (s *Service) CreateWorkflow(ctx context.Context, name, desc, graphJSON string) (*store.WorkflowDefinition, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "未命名编排"
	}
	if strings.TrimSpace(graphJSON) == "" {
		graphJSON = `{"nodes":[],"edges":[]}`
	}
	if _, err := ParseWorkflowGraph(graphJSON); err != nil {
		return nil, fmt.Errorf("invalid graph: %w", err)
	}
	id, err := s.Store.CreateWorkflowDefinition(ctx, name, desc, graphJSON)
	if err != nil {
		return nil, err
	}
	return s.Store.GetWorkflowDefinition(ctx, id)
}

// UpdateWorkflow 更新编排。
func (s *Service) UpdateWorkflow(ctx context.Context, id int64, name, desc, graphJSON string) (*store.WorkflowDefinition, error) {
	if strings.TrimSpace(graphJSON) == "" {
		return nil, fmt.Errorf("graph required")
	}
	if _, err := ParseWorkflowGraph(graphJSON); err != nil {
		return nil, fmt.Errorf("invalid graph: %w", err)
	}
	if err := s.Store.UpdateWorkflowDefinition(ctx, id, name, desc, graphJSON); err != nil {
		return nil, err
	}
	return s.Store.GetWorkflowDefinition(ctx, id)
}

// DeleteWorkflow 解散项目：先归档各员工协作记忆到工作区 projects/，再删定义。
func (s *Service) DeleteWorkflow(ctx context.Context, id int64) error {
	def, err := s.Store.GetWorkflowDefinition(ctx, id)
	if err != nil {
		return err
	}
	if def != nil {
		if archErr := s.ArchiveWorkflowProjectMemories(ctx, def); archErr != nil {
			log.Printf("workflow %d archive memories: %v", id, archErr)
		}
		_ = s.Store.DeleteWorkflowAgentSessions(ctx, id)
	}
	return s.Store.DeleteWorkflowDefinition(ctx, id)
}

// GetWorkflowRunDetail 运行详情 + 节点 executions。
func (s *Service) GetWorkflowRunDetail(ctx context.Context, runID int64) (map[string]any, error) {
	run, err := s.Store.GetWorkflowRun(ctx, runID)
	if err != nil || run == nil {
		return nil, fmt.Errorf("run not found")
	}
	def, _ := s.Store.GetWorkflowDefinition(ctx, run.DefinitionID)
	nes, _ := s.Store.ListNodeExecutions(ctx, runID)
	return map[string]any{
		"run":         run,
		"definition":  def,
		"executions":  nes,
	}, nil
}
