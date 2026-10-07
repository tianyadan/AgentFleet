package permission

import (
	"sync"
	"time"
)

// SessionGrant 会话内人工「继续允许」记录（精确 signature，不授权整类命令）。
type SessionGrant struct {
	ConversationID    int64
	ActionType        string
	CommandSignature  string
	WorkingDir        string
	Environment       string
	ResourceScope     string
	CreatedAt         time.Time
}

// SessionStore 会话授权持久化。
type SessionStore interface {
	Grant(convID int64, g SessionGrant) error
	Match(convID int64, sig string) (bool, error)
	Clear(convID int64) error
}

// SessionGrantFrom 由当前动作生成授权行。
func SessionGrantFrom(a ToolAction) SessionGrant {
	return SessionGrant{
		ConversationID:   a.ConversationID,
		ActionType:       a.ActionType,
		CommandSignature: a.Signature(),
		WorkingDir:       a.WorkingDir,
		Environment:      a.Environment,
		ResourceScope:    resourceScope(a),
		CreatedAt:        time.Now(),
	}
}

// MemorySessions 测试用内存实现。
type MemorySessions struct {
	mu   sync.Mutex
	byID map[int64][]SessionGrant
}

func NewMemorySessions() *MemorySessions {
	return &MemorySessions{byID: map[int64][]SessionGrant{}}
}

func (m *MemorySessions) Grant(convID int64, g SessionGrant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g.ConversationID = convID
	m.byID[convID] = append(m.byID[convID], g)
	return nil
}

func (m *MemorySessions) Match(convID int64, sig string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, g := range m.byID[convID] {
		if g.CommandSignature == sig {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemorySessions) Clear(convID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byID, convID)
	return nil
}
