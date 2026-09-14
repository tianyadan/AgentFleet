package testsrv

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type PendingCmd struct {
	ID       string    `json:"request_id"`
	ServerID int64     `json:"server_id"`
	Command  string    `json:"command"`
	Action   string    `json:"action"`
	Meaning  string    `json:"meaning"`
	Risk     string    `json:"risk"`
	Harmless bool      `json:"harmless"`
	ClientIP string    `json:"-"`
	Deadline time.Time `json:"deadline"`
}

type Hub struct {
	mu   sync.Mutex
	pend map[string]*PendingCmd
	wait time.Duration
}

func NewHub(waitSec int) *Hub {
	if waitSec < 30 {
		waitSec = 120
	}
	return &Hub{pend: map[string]*PendingCmd{}, wait: time.Duration(waitSec) * time.Second}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Hub) Register(serverID int64, action, command, meaning, risk string, harmless bool, clientIP string) *PendingCmd {
	h.mu.Lock()
	defer h.mu.Unlock()
	// 清理过期
	now := time.Now()
	for id, p := range h.pend {
		if now.After(p.Deadline) {
			delete(h.pend, id)
		}
	}
	p := &PendingCmd{
		ID: newID(), ServerID: serverID, Command: command, Action: action,
		Meaning: meaning, Risk: risk, Harmless: harmless, ClientIP: clientIP,
		Deadline: now.Add(h.wait),
	}
	h.pend[p.ID] = p
	return p
}

func (h *Hub) Take(id string) (*PendingCmd, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.pend[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(p.Deadline) {
		delete(h.pend, id)
		return nil, false
	}
	delete(h.pend, id)
	return p, true
}
