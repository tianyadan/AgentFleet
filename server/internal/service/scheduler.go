package service

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/robfig/cron/v3"
)

// Scheduler 管理型智能体定时任务。
type Scheduler struct {
	svc     *Service
	mu      sync.Mutex
	cron    *cron.Cron
	entries map[int64]cron.EntryID
}

// NewScheduler 创建并启动空调度器。
func NewScheduler(svc *Service) *Scheduler {
	c := cron.New()
	c.Start()
	return &Scheduler{svc: svc, cron: c, entries: map[int64]cron.EntryID{}}
}

// Reload 按数据库重建全部定时任务。
func (sch *Scheduler) Reload(ctx context.Context) {
	if sch == nil {
		return
	}
	items, err := sch.svc.Store.ListScheduledAgents(ctx)
	if err != nil {
		log.Printf("scheduler list: %v", err)
		return
	}
	sch.mu.Lock()
	defer sch.mu.Unlock()
	for id, eid := range sch.entries {
		sch.cron.Remove(eid)
		delete(sch.entries, id)
	}
	for _, a := range items {
		agent := a
		spec := strings.TrimSpace(agent.ScheduleCron)
		if spec == "" {
			continue
		}
		id := agent.ID
		eid, err := sch.cron.AddFunc(spec, func() {
			sch.runAgent(id)
		})
		if err != nil {
			log.Printf("scheduler add agent=%d cron=%q: %v", id, spec, err)
			continue
		}
		sch.entries[id] = eid
		log.Printf("scheduler armed agent=%d cron=%q label=%q", id, spec, agent.ScheduleLabel)
	}
}

func (sch *Scheduler) runAgent(agentID int64) {
	ctx := context.Background()
	a, err := sch.svc.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil || !a.ScheduleEnabled {
		return
	}
	if a.Status == "running" {
		log.Printf("scheduler skip agent=%d already running", agentID)
		return
	}
	if occ, _ := sch.svc.Store.GetOccupancy(ctx, agentID); occ != nil {
		log.Printf("scheduler skip agent=%d occupied by %s", agentID, occ.SourceType)
		return
	}
	q := "【定时任务】请按你的规则与权限策略执行例行工作,并简要汇报结果。"
	if a.ScheduleLabel != "" {
		q = "【定时任务·" + a.ScheduleLabel + "】请按你的规则与权限策略执行例行工作,并简要汇报结果。"
	}
	_, err = sch.svc.AskManaged(ctx, agentID, q, nil)
	if err != nil {
		log.Printf("scheduler ask agent=%d: %v", agentID, err)
	}
}

// Stop 停止调度。
func (sch *Scheduler) Stop() {
	if sch == nil || sch.cron == nil {
		return
	}
	ctx := sch.cron.Stop()
	<-ctx.Done()
}
