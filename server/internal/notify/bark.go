// Package notify 封装 Bark / ai-notify 推送(待授权提醒等)。
package notify

import (
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Notifier 异步调用本机通知脚本;按 request_id 去重。
type Notifier struct {
	bin     string
	enabled bool
	mu      sync.Mutex
	seen    map[string]time.Time
	runner  func(name string, args ...string) error // 可注入测试
}

// New 创建通知器。bin 为空或 enabled=false 时 PermissionOnce 直接跳过。
func New(bin string, enabled bool) *Notifier {
	if bin == "" {
		enabled = false
	}
	return &Notifier{
		bin:     bin,
		enabled: enabled,
		seen:    map[string]time.Time{},
		runner: func(name string, args ...string) error {
			cmd := exec.Command(name, args...)
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("notify script output: %s", string(out))
				return err
			}
			return nil
		},
	}
}

// PermissionOnce 对同一 requestID 只推一次。返回是否实际发起推送。
func (n *Notifier) PermissionOnce(requestID, message string) bool {
	if n == nil || !n.enabled || n.bin == "" || requestID == "" {
		return false
	}
	n.mu.Lock()
	if _, ok := n.seen[requestID]; ok {
		n.mu.Unlock()
		return false
	}
	n.seen[requestID] = time.Now()
	if len(n.seen) > 500 {
		n.seen = map[string]time.Time{requestID: time.Now()}
	}
	n.mu.Unlock()

	msg := message
	if msg == "" {
		msg = "有命令等待授权"
	}
	bin := n.bin
	go func() {
		if err := n.runner(bin, msg); err != nil {
			log.Printf("bark notify failed: %v", err)
		}
	}()
	return true
}
