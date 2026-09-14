package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// nodeEventBuf 节流写入节点 events_json。
type nodeEventBuf struct {
	mu       sync.Mutex
	events   []map[string]any
	dirty    bool
	execID   int64
	svc      *Service
	maxKeep  int
}

func newNodeEventBuf(svc *Service, execID int64) *nodeEventBuf {
	return &nodeEventBuf{svc: svc, execID: execID, maxKeep: 200}
}

func (b *nodeEventBuf) Add(ev map[string]any) {
	if b == nil || ev == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
	if len(b.events) > b.maxKeep {
		b.events = b.events[len(b.events)-b.maxKeep:]
	}
	b.dirty = true
}

func (b *nodeEventBuf) Flush(ctx context.Context) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if !b.dirty {
		b.mu.Unlock()
		return
	}
	raw, _ := json.Marshal(b.events)
	b.dirty = false
	b.mu.Unlock()
	_ = b.svc.Store.SetNodeExecutionEvents(ctx, b.execID, string(raw))
}

func (b *nodeEventBuf) StartFlusher(ctx context.Context) {
	go func() {
		t := time.NewTicker(400 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				b.Flush(context.WithoutCancel(ctx))
				return
			case <-t.C:
				b.Flush(ctx)
			}
		}
	}()
}

// askEventToNodeEvent 压缩 Ask SSE 为节点面板事件（去掉大段 chunk 杂音，保留 activity/命令类）。
func askEventToNodeEvent(ev map[string]any) map[string]any {
	if ev == nil {
		return nil
	}
	typ, _ := ev["type"].(string)
	switch typ {
	case "activity":
		return map[string]any{
			"type": "activity", "tool": ev["tool"], "summary": ev["summary"],
			"at": time.Now().UnixMilli(),
		}
	case "system_note":
		return map[string]any{
			"type": "system", "summary": ev["content"], "at": time.Now().UnixMilli(),
		}
	case "error":
		return map[string]any{
			"type": "error", "summary": ev["message"], "at": time.Now().UnixMilli(),
		}
	case "chunk":
		// 仅记录「有正文输出」提示，不堆全文
		return map[string]any{
			"type": "output", "summary": "正在输出内容…", "at": time.Now().UnixMilli(),
		}
	default:
		return nil
	}
}
