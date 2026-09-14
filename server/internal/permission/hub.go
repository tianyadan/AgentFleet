package permission

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"
)

// Request 一次待裁决的工具授权请求。
type Request struct {
	ID        string
	ToolName  string
	Summary   string
	Note      string
	Input     map[string]interface{}
	ConvID    int64
	ClientIP  string
	Meaning   string // AI 解释的命令含义
	Risk      string // AI 风险评估 low/mid/high
	CreatedAt time.Time
	Deadline  time.Time

	ch   chan Decision
	once sync.Once
}

// Cmd 该请求若为 Bash,记录其命令(用于「已授权命令」清单)。
func (r *Request) Cmd() string {
	if r.Summary == "" {
		return ""
	}
	return r.Summary
}

// Public 是对外可见的请求视图。
type Public struct {
	ID        string `json:"request_id"`
	ToolName  string `json:"tool_name"`
	Summary   string `json:"summary"`
	Note      string `json:"note,omitempty"`
	Meaning   string `json:"meaning,omitempty"` // AI 解释的命令含义(仅命令弹窗)
	Risk      string `json:"risk,omitempty"`    // AI 风险评估 low/mid/high
	Decision  string `json:"decision,omitempty"`   // allow|deny(命令轨迹)
	DecidedBy string `json:"decided_by,omitempty"` // user|ai|system|...
	ConvID    int64  `json:"conversation_id"`
	ClientIP  string `json:"client_ip"`
	CreatedAt string `json:"created_at"`
	Deadline  string `json:"deadline"`
}

// Resolved 一次授权裁决的落点。
type Resolved struct {
	ID       string `json:"request_id"`
	Behavior string `json:"behavior"`
	By       string `json:"by"`
}

// Event 推送到 SSE 的授权事件。
type Event struct {
	Kind string    `json:"kind"`
	Req  *Public   `json:"req,omitempty"`
	Res  *Resolved `json:"res,omitempty"`
}

// ErrUnknown 请求不存在或已处理。
var ErrUnknown = errors.New("permission: unknown or resolved request")

// Hub 维护挂起中的授权请求,并在前端回传时唤醒等待的 hook。
// 同时保存会话级「AI 自动审核」开关与审核结论缓存(均为进程内状态,重启即失效,
// 缓存随会话结束清理,不跨会话复用)。
type Hub struct {
	mu           sync.Mutex
	m            map[string]*Request
	waitSec      int
	subs         map[chan Event]struct{}
	auto         map[int64]bool        // convID -> 是否开启 AI 审核
	allowAllNoRm map[int64]bool        // convID -> 编排级放行(除 rm)
	cache        map[string]cacheEntry // convID|tool|input -> 审核结论
	cmds         []string              // 已授权/已执行的 Bash 命令(进程内,环形上限)
}

type cacheEntry struct {
	allow  bool
	reason string
}

// AppendCommand 记录一条已授权/已执行的 Bash 命令(进程内,上限 200,超出滚动)。
// AppendCommand 记录一条已授权/已执行的 Bash 命令(进程内,上限 200,超出滚动)。
// AppendCommand 在授权通过(Bash)时自动记录命令。
// Commands 返回已授权/已执行的命令副本。
// AppendCommand 记录一条已授权/已执行的 Bash 命令(进程内,上限 200,超出滚动)。
// Commands 返回已授权/已执行的命令副本。
// AppendCommand 记录一条已授权/已执行的 Bash 命令(进程内,上限 200,超出滚动)。
// Commands 返回已授权/已执行的命令副本。
// AppendCommand 记录一条已授权/已执行的 Bash 命令(进程内,上限 200,超出滚动)。
// Commands 返回已授权/已执行的命令副本。
// AppendCommand 记录一条已授权/已执行的 Bash 命令(进程内,上限 200,超出滚动)。
// Commands 返回已授权/已执行的命令副本。
// NewHub 创建中枢;waitSec<=0 时用 120s 默认。
func NewHub(waitSec int) *Hub {
	if waitSec <= 0 {
		waitSec = 120
	}
	return &Hub{
		m:            map[string]*Request{},
		waitSec:      waitSec,
		subs:         map[chan Event]struct{}{},
		auto:         map[int64]bool{},
		allowAllNoRm: map[int64]bool{},
		cache:        map[string]cacheEntry{},
	}
}

// maxCache 审核结论缓存上限;超出即整体清空(缓存只是省一次审核调用,清掉不影响正确性)。
const maxCache = 2000

// SetAuto 开关某会话的 AI 自动审核。
func (h *Hub) SetAuto(convID int64, enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if enabled {
		h.auto[convID] = true
		return
	}
	delete(h.auto, convID)
}

// IsAuto 返回该会话是否开启 AI 自动审核。
func (h *Hub) IsAuto(convID int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.auto[convID]
}

// SetAllowAllExceptRm 编排级：除 rm 外自动放行工具权限弹窗。
func (h *Hub) SetAllowAllExceptRm(convID int64, enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if enabled {
		h.allowAllNoRm[convID] = true
		return
	}
	delete(h.allowAllNoRm, convID)
}

// IsAllowAllExceptRm 是否开启编排级放行。
func (h *Hub) IsAllowAllExceptRm(convID int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.allowAllNoRm[convID]
}

// cachedVerdict 查审核缓存。
func (h *Hub) CachedVerdict(key string) (allow bool, reason string, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.cache[key]
	return e.allow, e.reason, ok
}

// putCached 写入审核缓存(allow/deny 都缓存,deny 命中仍会退回弹窗,只是省一次调用)。
func (h *Hub) PutCached(key string, allow bool, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.cache) >= maxCache {
		h.cache = map[string]cacheEntry{}
	}
	h.cache[key] = cacheEntry{allow: allow, reason: reason}
}

// ReviewKey 由会话 + 工具 + 规范化入参派生,保证同会话同调用命中同一缓存。
func ReviewKey(convID int64, tool string, input map[string]interface{}) string {
	b, _ := json.Marshal(input)
	dig := sha256.New()
	dig.Write([]byte(tool))
	dig.Write(b)
	sum := dig.Sum(nil)
	return strconv.FormatInt(convID, 10) + "|" + hex.EncodeToString(sum[:8])
}

// WaitTimeout 返回授权等待上限。
func (h *Hub) WaitTimeout() time.Duration { return time.Duration(h.waitSec) * time.Second }

// newID 生成 request_id。
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		n := time.Now().UnixNano()
		return "perm_" + hex.EncodeToString([]byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)})
	}
	return "perm_" + hex.EncodeToString(b[:])
}

// Register 登记一个新请求(分类器已判定为 ask),并广播给订阅者。
func (h *Hub) Register(tool string, input map[string]interface{}, summary, note string, convID int64, clientIP string) *Request {
	r := &Request{
		ID:        newID(),
		ToolName:  tool,
		Summary:   summary,
		Note:      note,
		Input:     input,
		ConvID:    convID,
		ClientIP:  clientIP,
		CreatedAt: time.Now(),
		ch:        make(chan Decision, 1),
	}
	r.Deadline = r.CreatedAt.Add(h.WaitTimeout())
	p := r.public()

	h.mu.Lock()
	if len(h.m) < 500 {
		h.m[r.ID] = r
	}
	h.mu.Unlock()
	h.broadcast(Event{Kind: "request", Req: &p})
	return r
}

// AutoApprove 记录一次由 AI 审核直接放行的授权(不弹窗),仅用于前端展示轨迹。
func (h *Hub) AutoApprove(tool, summary, reason string, convID int64, clientIP string) {
	h.broadcast(Event{Kind: "auto", Req: &Public{
		ID:        newID(),
		ToolName:  tool,
		Summary:   summary,
		Note:      "AI 审核放行:" + reason,
		ConvID:    convID,
		ClientIP:  clientIP,
		CreatedAt: time.Now().Format(time.RFC3339),
	}})
}

// broadcast 非阻塞推送给所有订阅者。
func (h *Hub) broadcast(ev Event) {
	h.mu.Lock()
	subs := make([]chan Event, 0, len(h.subs))
	for ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Wait 阻塞直到裁决、超时或 ctx 取消。超时/取消按拒绝处理。
func (h *Hub) Wait(ctx context.Context, id string) Decision {
	r := h.get(id)
	if r == nil {
		return Decision{Deny, "授权请求已失效"}
	}
	timer := time.NewTimer(time.Until(r.Deadline))
	defer timer.Stop()

	select {
	case d := <-r.ch:
		h.remove(id)
		h.broadcastResolved(Resolved{id, string(d.Behavior), "user"})
		return d
	case <-timer.C:
		h.remove(id)
		h.broadcastResolved(Resolved{id, "deny", "timeout"})
		return Decision{Deny, "授权超时,已自动拒绝"}
	case <-ctx.Done():
		h.remove(id)
		h.broadcastResolved(Resolved{id, "deny", "disconnect"})
		return Decision{Deny, "会话已结束,已自动拒绝"}
	}
}

// Decide 由前端调用,唤醒等待中的 hook。重复调用幂等。
func (h *Hub) Decide(id string, b Behavior, reason string) error {
	r := h.get(id)
	if r == nil {
		return ErrUnknown
	}
	var d = Decision{b, reason}
	first := false
	r.once.Do(func() {
		first = true
		select {
		case r.ch <- d:
		default:
		}
	})
	if first {
		h.remove(id)
		h.broadcastResolved(Resolved{id, string(b), "user"})
	}
	return nil
}

// Pending 返回挂起中的请求快照(convID=0 表示全部)。
func (h *Hub) Pending(convID int64) []Public {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []Public{}
	for _, r := range h.m {
		if convID == 0 || r.ConvID == convID {
			out = append(out, r.public())
		}
	}
	return out
}

// DropByConv 清理某会话的全部挂起请求(SSE 断开/会话结束),按拒绝唤醒。
func (h *Hub) DropByConv(convID int64) {
	h.drop(func(r *Request) bool { return r.ConvID == convID }, "连接已断开,已自动拒绝")
}

// drop 按条件拒绝并唤醒等待中的 hook。
func (h *Hub) drop(match func(*Request) bool, reason string) {
	h.mu.Lock()
	var ids []string
	for id, r := range h.m {
		if match(r) {
			ids = append(ids, id)
		}
	}
	h.mu.Unlock()
	for _, id := range ids {
		if r := h.get(id); r != nil {
			done := false
			r.once.Do(func() {
				done = true
				select {
				case r.ch <- Decision{Deny, reason}:
				default:
				}
			})
			if done {
				h.remove(id)
				h.broadcastResolved(Resolved{id, "deny", "disconnect"})
			}
		}
	}
}

// CloseAll 进程退出时清空(全部按拒绝)。
func (h *Hub) CloseAll() {
	h.mu.Lock()
	ids := make([]string, 0, len(h.m))
	for id := range h.m {
		ids = append(ids, id)
	}
	h.mu.Unlock()
	for _, id := range ids {
		if r := h.get(id); r != nil {
			r.once.Do(func() {
				select {
				case r.ch <- Decision{Deny, "服务已停止,已自动拒绝"}:
				default:
				}
			})
			h.remove(id)
		}
	}
}

// Subscribe 返回接收授权事件的通道;用完须 Unsubscribe。
func (h *Hub) Subscribe() chan Event {
	ch := make(chan Event, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe 退订并关闭通道。
func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *Hub) broadcastResolved(res Resolved) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- Event{Kind: "resolved", Res: &res}:
		default:
		}
	}
}

func (h *Hub) get(id string) *Request {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.m[id]
}

func (h *Hub) remove(id string) {
	h.mu.Lock()
	delete(h.m, id)
	h.mu.Unlock()
}

func (r *Request) public() Public {
	return Public{
		ID:        r.ID,
		ToolName:  r.ToolName,
		Summary:   r.Summary,
		Note:      r.Note,
		Meaning:   r.Meaning,
		Risk:      r.Risk,
		ConvID:    r.ConvID,
		ClientIP:  r.ClientIP,
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		Deadline:  r.Deadline.Format(time.RFC3339),
	}
}

// BroadcastCommand 广播一条命令轨迹(允许或拒绝),用于对话流展示。
func (h *Hub) BroadcastCommand(tool, summary, decision, by, risk, meaning string, convID int64) {
	if summary == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- Event{Kind: "command", Req: &Public{
			ToolName: tool, Summary: summary, Decision: decision, DecidedBy: by,
			Risk: risk, Meaning: meaning, ConvID: convID,
		}}:
		default:
		}
	}
}

// AppendCommand 记录一条已授权/已执行的 Bash 命令(进程内,上限 200,超出滚动)。
func (h *Hub) AppendCommand(cmd string) {
	if cmd == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cmds = append(h.cmds, cmd)
	if len(h.cmds) > 200 {
		h.cmds = h.cmds[len(h.cmds)-200:]
	}
}

// Commands 返回已授权/已执行的命令副本。
func (h *Hub) Commands() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.cmds))
	copy(out, h.cmds)
	return out
}
