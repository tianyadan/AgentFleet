import { useEffect, useMemo, useRef, useState } from 'react'
import AgentMarkdown from './agentMarkdown.jsx'
import {
  allPendingPerms,
  anyAgentLoading,
  clearAgentSession,
  getAgentSession,
  patchAgentSession,
  setAgentChatLog,
  subscribeAgentSessions,
} from './managedAgentSessions.js'

const API = '/api'
const PAGE_SIZE = 20
const ENGINES = [
  { id: 'claude', label: 'Claude', color: '#D97757' },
  { id: 'codex', label: 'Codex', color: '#10a37f' },
  { id: 'agent', label: 'Cursor', color: '#6366f1' },
]

/** 运行中轮换文案（参考 Claude thinking，夹杂一点幽默） */
const THINKING_PHRASES = [
  'thinking',
  'pondering',
  'brewing ideas',
  'consulting the rubber duck',
  'untangling spaghetti',
  'asking the void politely',
  'compiling vibes',
  'negotiating with reality',
  'warming up neurons',
  'reading tea leaves',
  'sharpening virtual pencils',
  'counting token sheep',
  'channeling inner Claude',
  'politely poking the model',
  'assembling coherent thoughts',
]

const SCHEDULE_PRESETS = [
  { id: '', label: '自定义 cron', cron: '' },
  { id: 'daily9', label: '每天 09:00', cron: '0 9 * * *' },
  { id: 'daily18', label: '每天 18:00', cron: '0 18 * * *' },
  { id: 'weekday9', label: '工作日 09:00', cron: '0 9 * * 1-5' },
  { id: 'weekly1', label: '每周一 09:00', cron: '0 9 * * 1' },
  { id: 'monthly1', label: '每月 1 日 09:00', cron: '0 9 1 * *' },
  { id: 'hourly', label: '每小时整点', cron: '0 * * * *' },
]

function engineMeta(id) {
  return ENGINES.find((e) => e.id === id) || { id, label: id, color: '#94a3b8' }
}

function emptyPolicy() {
  return {
    allow_write: true,
    allow_network: true,
    allow_rm: false,
    allow_browser: false,
    workspace_bound: false,
    workspace_path: '',
    schedule_enabled: false,
    schedule_cron: '',
    schedule_label: '',
    schedule_preset: '',
  }
}

/** 状态英文 → 中文展示 */
function statusZh(st) {
  const s = String(st || 'idle')
  if (s === 'running' || s === 'busy' || s === '插话中') return '忙碌中'
  if (s === 'waiting') return '等待中'
  if (s === 'error') return '错误'
  if (s === 'done') return '空闲中'
  return '空闲中'
}

/** 是否被团队编排占用 */
function isWorkflowBusy(a) {
  const o = a?.occupancy
  if (!o) return false
  return o.source_type === 'WORKFLOW' || Number(o.workflow_run_id) > 0
}

/** 忙碌来源一行文案 */
function occupancyLine(a) {
  const o = a?.occupancy
  if (!o) return ''
  const team = o.source_name || o.source_type || ''
  const task = o.task_name || ''
  return `${team}${task ? ` · ${task}` : ''}`
}

function statusClass(st) {
  const s = String(st || 'idle')
  if (s === 'running' || s === 'busy' || s === '插话中') return 'running'
  if (s === 'waiting') return 'waiting'
  if (s === 'error') return 'error'
  return 'idle'
}

/**
 * 管理台「智能体管理」：授权弹窗、自动审核、分页、插话、设置。
 */
export default function ManagedAgentsPanel({ authHeaders, onUnauthorized, active = true, onOpenWorkflowRun }) {
  const [agents, setAgents] = useState([])
  const [selectedId, setSelectedId] = useState(0)
  const [question, setQuestion] = useState('')
  const [tasks, setTasks] = useState([])
  const [createOpen, setCreateOpen] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [folders, setFolders] = useState([])
  const [agentSearch, setAgentSearch] = useState('')
  const [collapsedFolders, setCollapsedFolders] = useState({})
  const [folderCreateOpen, setFolderCreateOpen] = useState(false)
  const [folderCreateName, setFolderCreateName] = useState('新建文件夹')
  const [folderEdit, setFolderEdit] = useState(null) // { id, name }
  const [folderEditName, setFolderEditName] = useState('')
  const [newName, setNewName] = useState('开发智能体')
  const [newEngine, setNewEngine] = useState('claude')
  const [newRules, setNewRules] = useState('')
  const [newPolicy, setNewPolicy] = useState(() => emptyPolicy())
  const [policyDraft, setPolicyDraft] = useState(() => emptyPolicy())
  const [workspaces, setWorkspaces] = useState([])
  const [mentionOpen, setMentionOpen] = useState(false)
  const [mentionIdx, setMentionIdx] = useState(0)
  const composerRef = useRef(null)
  const [busy, setBusy] = useState(false)
  const [renameDraft, setRenameDraft] = useState('')
  const [rulesDraft, setRulesDraft] = useState('')
  const [permBusy, setPermBusy] = useState(false)
  const [autoSaving, setAutoSaving] = useState(false)
  const [, setTick] = useState(0)
  const [, setSessVer] = useState(0)
  const boxRef = useRef(null)
  const stickBottomRef = useRef(true)
  const viewRef = useRef({ active, selectedId, agents })
  viewRef.current = { active, selectedId: selectedId, agents }

  const selected = agents.find((a) => a.id === selectedId) || null
  const sess = getAgentSession(selectedId)
  const loading = !!sess.loading
  const compressing = !!sess.compressing
  const chatLog = useMemo(() => sess.chatLog || [], [sess.chatLog])
  const interrupting = !!sess.interrupting
  const permQueue = sess.permQueue || []
  const autoOn = !!sess.autoOn
  const invokeMap = sess.invokes || {}
  const mentionCandidates = agents.filter((a) => a.id !== selectedId)

  useEffect(() => subscribeAgentSessions(() => setSessVer((v) => v + 1)), [])
  useEffect(() => {
    const t = setInterval(() => setTick((x) => x + 1), 1000)
    return () => clearInterval(t)
  }, [])

  // 上下文圆环：空闲/运行中每 5s 刷新真实占用
  useEffect(() => {
    if (!selectedId || !active) return undefined
    refreshAgentContext(selectedId, false)
    const t = setInterval(() => refreshAgentContext(selectedId, false), 5000)
    return () => clearInterval(t)
  }, [selectedId, active])

  // 授权倒计时本地出队
  useEffect(() => {
    if (permQueue.length === 0) return
    const t = setInterval(() => {
      patchAgentSession(selectedId, {
        permQueue: (getAgentSession(selectedId).permQueue || []).filter((p) => remainSec(p) > 0),
      })
    }, 1000)
    return () => clearInterval(t)
  }, [selectedId, permQueue.length])

  async function loadAgents(preferId) {
    const [res, folderRes] = await Promise.all([
      fetch(`${API}/admin/agents`, { headers: authHeaders() }),
      fetch(`${API}/admin/agent-folders`, { headers: authHeaders() }),
    ])
    if (res.status === 401 || folderRes.status === 401) { onUnauthorized?.(); return }
    if (!res.ok) return
    const d = await res.json()
    const items = d.items || []
    setAgents(items)
    if (folderRes.ok) {
      const fd = await folderRes.json()
      setFolders(fd.items || [])
    }
    // 从服务端恢复自动审核开关(刷新不丢)
    items.forEach((a) => {
      if (a && a.id) patchAgentSession(a.id, { autoOn: !!a.auto_review })
    })
    const next = preferId || selectedId || items[0]?.id || 0
    if (next && items.some((a) => a.id === next)) setSelectedId(next)
    else if (items[0]) setSelectedId(items[0].id)
    else setSelectedId(0)
  }

  function mapMsgs(items) {
    return (items || []).map((m) => {
      let meta = null
      if (m.meta_json) {
        try { meta = typeof m.meta_json === 'string' ? JSON.parse(m.meta_json) : m.meta_json } catch { /* ignore */ }
      }
      const base = {
        id: m.id,
        role: m.role,
        content: m.content,
        status: m.status,
        time: m.created_at ? new Date(m.created_at).getTime() : 0,
        replySec: m.reply_ms ? Math.round(m.reply_ms / 1000) : null,
        note: m.note,
        calleeId: meta?.callee_id,
        calleeName: meta?.callee_name,
        callerName: meta?.caller_name,
        collapsed: m.role === 'invoke_quote',
      }
      if (m.role === 'command' && typeof m.content === 'string' && m.content.trim().startsWith('{')) {
        try {
          const cmd = JSON.parse(m.content)
          return {
            ...base,
            content: cmd.command || m.content,
            decision: cmd.decision || (m.status === 'denied' ? 'deny' : 'allow'),
            decidedBy: cmd.by || '',
            risk: cmd.risk || '',
            meaning: cmd.meaning || '',
            note: cmd.note || m.note,
          }
        } catch { /* plain text */ }
      }
      if (m.role === 'command' && m.status === 'denied') {
        return { ...base, decision: 'deny' }
      }
      return base
    })
  }

  /** 不在该 agent 详情页时请求 Bark(策略 A) */
  function maybeBarkNotify(agentId, ev) {
    const v = viewRef.current
    if (v.active && v.selectedId === agentId) return
    const agent = (v.agents || []).find((a) => a.id === agentId)
    fetch(`${API}/admin/agents/notify-permission`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        request_id: ev.request_id,
        agent_id: agentId,
        agent_name: agent?.name || `智能体#${agentId}`,
        tool_name: ev.tool_name,
        command: ev.summary,
        risk: ev.risk,
        meaning: ev.meaning,
      }),
    }).catch(() => {})
  }

  async function loadDetail(id, { force = false } = {}) {
    if (!id) { setTasks([]); return }
    const local = getAgentSession(id)
    const [msgRes, taskRes] = await Promise.all([
      fetch(`${API}/admin/agents/${id}/messages?limit=${PAGE_SIZE}`, { headers: authHeaders() }),
      fetch(`${API}/admin/agents/${id}/tasks`, { headers: authHeaders() }),
    ])
    if (msgRes.status === 401 || taskRes.status === 401) { onUnauthorized?.(); return }
    const msgData = await msgRes.json()
    const taskData = await taskRes.json()
    setTasks(taskData.items || [])
    // force:结束后用 DB 校准全文;非 force 时运行中不覆盖本地流式气泡
    if (force || !getAgentSession(id).loading) {
      setAgentChatLog(id, mapMsgs(msgData.items))
      patchAgentSession(id, {
        totalMessages: msgData.total || 0,
        hasMore: !!msgData.has_more,
        convId: msgData.conversation_id || 0,
      })
    }
    // 补齐挂起授权
    const conv = msgData.conversation_id || local.convId
    if (conv) {
      fetch(`${API}/permissions/pending?conversation_id=${conv}`)
        .then((r) => r.json())
        .then((d) => {
          const items = d.items || []
          if (items.length) {
            patchAgentSession(id, {
              permQueue: dedupPerm([...(getAgentSession(id).permQueue || []), ...items]),
            })
          }
        })
        .catch(() => {})
    }
  }

  async function loadOlder() {
    if (!selectedId || sess.loadingMore || !sess.hasMore || loading) return
    const oldestId = chatLog.find((m) => m.id)?.id
    if (!oldestId) return
    const el = boxRef.current
    const prevH = el ? el.scrollHeight : 0
    patchAgentSession(selectedId, { loadingMore: true })
    try {
      const res = await fetch(
        `${API}/admin/agents/${selectedId}/messages?before_id=${oldestId}&limit=${PAGE_SIZE}`,
        { headers: authHeaders() },
      )
      if (res.status === 401) { onUnauthorized?.(); return }
      const d = await res.json()
      const older = mapMsgs(d.items)
      setAgentChatLog(selectedId, (prev) => {
        const ids = new Set(prev.map((m) => m.id).filter(Boolean))
        const merged = [...older.filter((m) => !ids.has(m.id)), ...prev]
        return merged
      })
      patchAgentSession(selectedId, {
        hasMore: !!d.has_more,
        totalMessages: d.total || sess.totalMessages,
      })
      requestAnimationFrame(() => {
        if (el) el.scrollTop = el.scrollHeight - prevH
      })
    } finally {
      patchAgentSession(selectedId, { loadingMore: false })
    }
  }

  useEffect(() => { loadAgents() /* eslint-disable-next-line */ }, [])

  useEffect(() => {
    fetch(`${API}/workspaces`)
      .then((r) => r.json())
      .then((d) => setWorkspaces((d.items || d || []).filter((w) => (w.type || 'code') === 'code')))
      .catch(() => {})
  }, [])

  useEffect(() => {
    if (!selected) {
      setRenameDraft(''); setRulesDraft(''); setTasks([])
      return
    }
    setRenameDraft(selected.name || '')
    setRulesDraft(selected.rules_prompt || '')
    setPolicyDraft(policyFromAgent(selected))
    loadDetail(selected.id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId])

  useEffect(() => {
    if (!active || !stickBottomRef.current) return
    const el = boxRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [chatLog, loading, active, permQueue.length])

  useEffect(() => {
    const tick = () => {
      loadAgents(selectedId || undefined)
      if (selectedId) {
        fetch(`${API}/admin/agents/${selectedId}/tasks`, { headers: authHeaders() })
          .then((r) => r.json())
          .then((d) => setTasks(d.items || []))
          .catch(() => {})
      }
    }
    const ms = anyAgentLoading() || agents.some((a) => a.status === 'running') ? 1500 : 4000
    const t = setInterval(tick, ms)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId, agents.map((a) => a.status).join(',')])

  function onChatScroll() {
    const el = boxRef.current
    if (!el) return
    stickBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48
    if (el.scrollTop < 40) loadOlder()
  }

  async function createAgent(e) {
    e?.preventDefault?.()
    if (busy) return
    setBusy(true)
    try {
      const res = await fetch(`${API}/admin/agents`, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({
          name: newName.trim() || '未命名智能体',
          engine: newEngine,
          rules_prompt: newRules,
          allow_write: !!newPolicy.allow_write,
          allow_network: !!newPolicy.allow_network,
          allow_rm: !!newPolicy.allow_rm,
          allow_browser: !!newPolicy.allow_browser,
          workspace_path: newPolicy.workspace_bound ? (newPolicy.workspace_path || '') : '',
          schedule_enabled: !!newPolicy.schedule_enabled,
          schedule_cron: newPolicy.schedule_enabled ? (newPolicy.schedule_cron || '') : '',
          schedule_label: newPolicy.schedule_enabled ? (newPolicy.schedule_label || '') : '',
        }),
      })
      if (res.status === 401) { onUnauthorized?.(); return }
      const d = await res.json()
      if (!res.ok) { patchAgentSession(selectedId, { error: d.error || '创建失败' }); return }
      setCreateOpen(false)
      setNewPolicy(emptyPolicy())
      await loadAgents(d.id)
    } finally { setBusy(false) }
  }

  async function saveRules() {
    if (!selected || busy) return
    setBusy(true)
    try {
      const res = await fetch(`${API}/admin/agents/${selected.id}`, {
        method: 'PATCH',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({
          name: renameDraft.trim() || selected.name,
          rules_prompt: rulesDraft,
          allow_write: !!policyDraft.allow_write,
          allow_network: !!policyDraft.allow_network,
          allow_rm: !!policyDraft.allow_rm,
          allow_browser: !!policyDraft.allow_browser,
          workspace_path: policyDraft.workspace_bound ? (policyDraft.workspace_path || '') : '',
          schedule_enabled: !!policyDraft.schedule_enabled,
          schedule_cron: policyDraft.schedule_enabled ? (policyDraft.schedule_cron || '') : '',
          schedule_label: policyDraft.schedule_enabled ? (policyDraft.schedule_label || '') : '',
        }),
      })
      if (res.status === 401) { onUnauthorized?.(); return }
      setSettingsOpen(false)
      await loadAgents(selected.id)
    } finally { setBusy(false) }
  }

  async function createFolder(e) {
    e?.preventDefault?.()
    if (busy) return
    const name = folderCreateName.trim() || '新建文件夹'
    setBusy(true)
    try {
      const res = await fetch(`${API}/admin/agent-folders`, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ name }),
      })
      if (res.status === 401) { onUnauthorized?.(); return }
      if (!res.ok) return
      setFolderCreateOpen(false)
      setFolderCreateName('新建文件夹')
      await loadAgents(selectedId)
    } finally { setBusy(false) }
  }

  async function saveFolderEdit(e) {
    e?.preventDefault?.()
    if (!folderEdit || busy) return
    const name = folderEditName.trim()
    if (!name) return
    setBusy(true)
    try {
      const res = await fetch(`${API}/admin/agent-folders/${folderEdit.id}`, {
        method: 'PATCH',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ name }),
      })
      if (res.status === 401) { onUnauthorized?.(); return }
      if (!res.ok) return
      setFolderEdit(null)
      await loadAgents(selectedId)
    } finally { setBusy(false) }
  }

  async function moveAgentToFolder(agentId, folderId) {
    const res = await fetch(`${API}/admin/agents/${agentId}/folder`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ folder_id: folderId || 0 }),
    })
    if (res.status === 401) { onUnauthorized?.(); return }
    await loadAgents(selectedId)
  }

  /** 从文件夹移出到未分组 */
  async function removeAgentFromFolder(agentId) {
    await moveAgentToFolder(agentId, 0)
  }

  async function fetchStatusSnapshot() {
    if (!selected || loading) return
    const res = await fetch(`${API}/admin/agents/${selected.id}/status`, { headers: authHeaders() })
    if (res.status === 401) { onUnauthorized?.(); return }
    const d = await res.json()
    if (!res.ok) return
    setAgentChatLog(selected.id, (prev) => [
      ...prev,
      { role: 'status', content: d.text || '', time: Date.now() },
    ])
    stickBottomRef.current = true
    await refreshAgentContext(selected.id, true)
  }

  async function deleteAgent() {
    if (!selected || busy) return
    if (!window.confirm(`确认删除智能体「${selected.name}」？`)) return
    setBusy(true)
    try {
      const id = selected.id
      await fetch(`${API}/admin/agents/${id}`, { method: 'DELETE', headers: authHeaders() })
      clearAgentSession(id)
      await loadAgents(0)
    } finally { setBusy(false) }
  }

  async function newChat() {
    if (!selected || loading) return
    const res = await fetch(`${API}/admin/agents/${selected.id}/new`, { method: 'POST', headers: authHeaders() })
    const d = await res.json().catch(() => ({}))
    setAgentChatLog(selected.id, [])
    patchAgentSession(selected.id, {
      totalMessages: 0, hasMore: false, convId: d.conversation_id || 0, autoOn: false, permQueue: [],
      taskTokens: 0, contextUsed: 0, contextWindow: 0,
    })
    await loadAgents(selected.id)
    await refreshAgentContext(selected.id, true)
  }

  async function compressChat() {
    if (!selected || loading || compressing) return
    const id = selected.id
    patchAgentSession(id, { compressing: true, error: '' })
    try {
      const res = await fetch(`${API}/admin/agents/${id}/compress`, { method: 'POST', headers: authHeaders() })
      const d = await res.json().catch(() => ({}))
      if (!res.ok) { patchAgentSession(id, { error: d.error || '压缩失败' }); return }
      await loadDetail(id, { force: true })
      await refreshAgentContext(id, true)
    } catch (err) {
      patchAgentSession(id, { error: String(err) })
    } finally {
      patchAgentSession(id, { compressing: false })
    }
  }

  async function toggleAuto(v) {
    if (!selected || autoSaving) return
    setAutoSaving(true)
    try {
      let conv = getAgentSession(selected.id).convId || selected.conversation_id
      if (!conv && v) {
        // 尚无会话时先建空会话
        const res = await fetch(`${API}/admin/agents/${selected.id}/new`, { method: 'POST', headers: authHeaders() })
        const d = await res.json()
        if (!res.ok) throw new Error(d.error || '创建会话失败')
        conv = d.conversation_id
        patchAgentSession(selected.id, { convId: conv })
        await loadAgents(selected.id)
      }
      if (!conv) { patchAgentSession(selected.id, { autoOn: false }); return }
      const res = await fetch(`${API}/permissions/auto`, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ conversation_id: conv, enabled: v }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || `自动审核开关失败(${res.status})`)
      }
      patchAgentSession(selected.id, { autoOn: v })
      if (!v) patchAgentSession(selected.id, { permQueue: [] })
    } catch (e) {
      patchAgentSession(selected.id, { autoOn: false, error: String(e.message || e) })
    } finally { setAutoSaving(false) }
  }

  async function decide(requestId, behavior) {
    if (permBusy) return
    setPermBusy(true)
    try {
      await fetch(`${API}/permissions/decide`, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ request_id: requestId, behavior }),
      })
    } finally {
      const q = (getAgentSession(selectedId).permQueue || []).filter((p) => p.request_id !== requestId)
      patchAgentSession(selectedId, { permQueue: q })
      setPermBusy(false)
    }
  }

  function parseSSE(text) {
    const events = []
    let rest = text
    while (true) {
      const idx = rest.indexOf('\n\n')
      if (idx < 0) break
      const block = rest.slice(0, idx)
      rest = rest.slice(idx + 2)
      const line = block.split('\n').find((l) => l.startsWith('data:'))
      if (!line) continue
      try { events.push(JSON.parse(line.slice(5).trim())) } catch { /* ignore */ }
    }
    return { events, rest }
  }

  function applyEvents(agentId, events) {
    for (const ev of events) {
      if (ev.type === 'activity') {
        patchAgentSession(agentId, {
          liveActivity: { tool: ev.tool || '', summary: ev.summary || '运行中…', at: Date.now() },
        })
      } else if (ev.type === 'chunk') {
        const piece = (ev.content || '').replace(/\\n/g, '\n')
        if (!piece) continue
        setAgentChatLog(agentId, (prev) => {
          const n = [...prev]
          // 命令/引用事件会插在末尾;后续正文必须接到 assistant
          if (n.length && n[n.length - 1].role === 'assistant') {
            const last = n[n.length - 1]
            n[n.length - 1] = { ...last, content: (last.content || '') + piece }
          } else {
            n.push({ role: 'assistant', content: piece, time: Date.now() })
          }
          return n
        })
      } else if (ev.type === 'error') {
        setAgentChatLog(agentId, (prev) => {
          const n = [...prev]
          if (n.length && n[n.length - 1].role === 'assistant') {
            n[n.length - 1] = { ...n[n.length - 1], content: `❌ ${ev.message || '出错'}`, error: true }
          } else {
            n.push({ role: 'assistant', content: `❌ ${ev.message || '出错'}`, error: true })
          }
          return n
        })
      } else if (ev.type === 'done') {
        if (ev.conversation_id) patchAgentSession(agentId, { convId: ev.conversation_id })
        if (ev.reply_ms) {
          setAgentChatLog(agentId, (prev) => {
            const n = [...prev]
            for (let i = n.length - 1; i >= 0; i--) {
              if (n[i].role === 'assistant') {
                n[i] = { ...n[i], replySec: Math.round(ev.reply_ms / 1000) }
                break
              }
            }
            return n
          })
        }
        // 折叠所有委托小窗
        const inv = { ...(getAgentSession(agentId).invokes || {}) }
        Object.keys(inv).forEach((k) => { inv[k] = { ...inv[k], collapsed: true } })
        patchAgentSession(agentId, { invokes: inv, liveActivity: null })
      } else if (ev.type === 'invoke_status') {
        const id = String(ev.callee_id)
        const inv = { ...(getAgentSession(agentId).invokes || {}) }
        inv[id] = {
          ...(inv[id] || {}),
          calleeId: ev.callee_id,
          name: ev.callee_name || inv[id]?.name || `#${ev.callee_id}`,
          status: ev.status || 'waiting',
          message: ev.message || '',
          collapsed: false,
        }
        patchAgentSession(agentId, { invokes: inv })
      } else if (ev.type === 'invoke_result' || ev.type === 'invoke_quote') {
        const id = String(ev.callee_id)
        const inv = { ...(getAgentSession(agentId).invokes || {}) }
        inv[id] = {
          ...(inv[id] || {}),
          calleeId: ev.callee_id,
          name: ev.callee_name || inv[id]?.name,
          status: ev.error ? 'error' : 'done',
          content: ev.content || inv[id]?.content || '',
          collapsed: !!ev.collapsed || ev.type === 'invoke_quote',
        }
        patchAgentSession(agentId, { invokes: inv })
        if (ev.type === 'invoke_quote' && ev.content) {
          setAgentChatLog(agentId, (prev) => [
            ...prev,
            {
              role: 'invoke_quote',
              content: ev.content,
              calleeId: ev.callee_id,
              calleeName: ev.callee_name,
              collapsed: true,
              time: Date.now(),
            },
          ])
        }
      } else if (ev.type === 'usage') {
        const turn = Number(ev.turn_tokens) || (
          Number(ev.input_tokens || 0) + Number(ev.output_tokens || 0)
          + Number(ev.cache_read_tokens || 0) + Number(ev.cache_write_tokens || 0)
        )
        const patch = {}
        if (turn > 0) patch.taskTokens = turn
        if (Number(ev.used_tokens) > 0) patch.contextUsed = Number(ev.used_tokens)
        if (Number(ev.window_tokens) > 0) patch.contextWindow = Number(ev.window_tokens)
        if (Object.keys(patch).length) patchAgentSession(agentId, patch)
      } else if (ev.type === 'task_progress') {
        // 触发 tasks 刷新由轮询承接
      } else if (ev.type === 'system_note') {
        setAgentChatLog(agentId, (prev) => [
          ...prev,
          { role: 'system', content: ev.content || '系统提示', time: Date.now() },
        ])
      } else if (ev.type === 'permission_request') {
        patchAgentSession(agentId, {
          permQueue: dedupPerm([...(getAgentSession(agentId).permQueue || []), ev]),
          liveActivity: {
            tool: ev.tool_name || 'Tool',
            summary: `等待授权 · ${(ev.summary || ev.tool_name || '').slice(0, 72)}`,
            at: Date.now(),
          },
        })
        maybeBarkNotify(agentId, ev)
      } else if (ev.type === 'permission_auto') {
        setAgentChatLog(agentId, (prev) => [
          ...prev,
          { role: 'auto', content: `${ev.tool_name} · ${(ev.summary || '').slice(0, 80)}`, note: ev.note },
        ])
        patchAgentSession(agentId, {
          liveActivity: {
            tool: ev.tool_name || 'Tool',
            summary: `自动放行 · ${(ev.summary || '').slice(0, 64)}`,
            at: Date.now(),
          },
        })
      } else if (ev.type === 'command') {
        setAgentChatLog(agentId, (prev) => [
          ...prev,
          {
            role: 'command',
            content: ev.summary || ev.tool_name,
            decision: ev.decision || 'allow',
            decidedBy: ev.decided_by || '',
            risk: ev.risk || '',
            meaning: ev.meaning || '',
            note: ev.note || ev.output || '',
            time: Date.now(),
          },
        ])
      } else if (ev.type === 'permission_resolved') {
        patchAgentSession(agentId, {
          permQueue: (getAgentSession(agentId).permQueue || []).filter((p) => p.request_id !== ev.request_id),
        })
      }
    }
  }

  async function refreshAgentContext(agentId, fresh = false) {
    if (!agentId) return
    try {
      const q = fresh ? '?fresh=1' : ''
      const res = await fetch(`${API}/admin/agents/${agentId}/context${q}`, { headers: authHeaders() })
      if (res.status === 401) { onUnauthorized?.(); return }
      if (!res.ok) return
      const d = await res.json()
      patchAgentSession(agentId, {
        contextUsed: Number(d.used_tokens) || 0,
        contextWindow: Number(d.window_tokens) || 0,
      })
    } catch { /* ignore */ }
  }

  async function sendQuestion(agentId, q) {
    const ac = new AbortController()
    patchAgentSession(agentId, {
      loading: true, error: '', runStartedAt: Date.now(), abort: ac, interrupting: false, invokes: {},
      taskTokens: 0,
      liveActivity: { tool: '', summary: '启动中…', at: Date.now() },
    })
    refreshAgentContext(agentId, true)
    setAgentChatLog(agentId, (prev) => [
      ...prev,
      { role: 'user', content: q, time: Date.now() },
    ])
    stickBottomRef.current = true
    try {
      const res = await fetch(`${API}/admin/agents/${agentId}/ask`, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ question: q }),
        signal: ac.signal,
      })
      if (res.status === 401) { onUnauthorized?.(); return }
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || `HTTP ${res.status}`)
      }
      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const parsed = parseSSE(buf)
        buf = parsed.rest
        applyEvents(agentId, parsed.events)
        // 流式期间刷新任务进度
        if (parsed.events.some((e) => e.type === 'task_progress' || e.type === 'invoke_status')) {
          fetch(`${API}/admin/agents/${agentId}/tasks`, { headers: authHeaders() })
            .then((r) => r.json())
            .then((d) => { if (selectedId === agentId) setTasks(d.items || []) })
            .catch(() => {})
        }
      }
      await loadAgents(agentId)
      await loadDetail(agentId, { force: true })
    } catch (err) {
      if (err?.name === 'AbortError') {
        const isInterrupt = !!getAgentSession(agentId).pendingInterrupt
        if (!isInterrupt) {
          setAgentChatLog(agentId, (prev) => {
            const n = [...prev]
            for (let i = n.length - 1; i >= 0; i--) {
              if (n[i].role === 'assistant') {
                n[i] = { ...n[i], content: (n[i].content || '') + '\n\n⏹ 已停止' }
                return n
              }
            }
            n.push({ role: 'assistant', content: '⏹ 已停止', error: true, time: Date.now() })
            return n
          })
        }
      } else {
        patchAgentSession(agentId, { error: String(err) })
      }
    } finally {
      const pending = getAgentSession(agentId).pendingInterrupt
      patchAgentSession(agentId, {
        loading: false, abort: null, runStartedAt: 0, pendingInterrupt: '', interrupting: false,
        liveActivity: null,
      })
      await refreshAgentContext(agentId, true)
      if (pending) await sendQuestion(agentId, pending)
    }
  }

  /** 输入 @ 时弹出智能体列表 */
  function onComposerChange(e) {
    const v = e.target.value
    setQuestion(v)
    const caret = e.target.selectionStart || v.length
    const before = v.slice(0, caret)
    const at = before.lastIndexOf('@')
    if (at >= 0 && !/\s/.test(before.slice(at + 1)) && !before.slice(at).includes('](')) {
      setMentionOpen(true)
      setMentionIdx(0)
    } else {
      setMentionOpen(false)
    }
  }

  function insertMention(agent) {
    const el = composerRef.current
    const v = question
    const caret = el?.selectionStart ?? v.length
    const before = v.slice(0, caret)
    const after = v.slice(caret)
    const at = before.lastIndexOf('@')
    const token = `@[${agent.name}](#agent:${agent.id}) `
    const next = (at >= 0 ? before.slice(0, at) : before) + token + after
    setQuestion(next)
    setMentionOpen(false)
    requestAnimationFrame(() => {
      if (!el) return
      el.focus()
      const pos = (at >= 0 ? at : caret) + token.length
      el.setSelectionRange(pos, pos)
    })
  }

  async function ask() {
    if (!selected || !question.trim()) return
    const q = question.trim()
    setQuestion('')
    const id = selected.id
    const s = getAgentSession(id)
    if (s.loading) {
      patchAgentSession(id, { pendingInterrupt: q, interrupting: true })
      s.abort?.abort()
      return
    }
    await sendQuestion(id, q)
  }

  async function stopRun() {
    if (!selected) return
    const id = selected.id
    patchAgentSession(id, { pendingInterrupt: '', interrupting: false })
    getAgentSession(id).abort?.abort()
    try {
      await fetch(`${API}/admin/agents/${id}/stop`, { method: 'POST', headers: authHeaders() })
    } catch { /* ignore */ }
    patchAgentSession(id, { loading: false, abort: null, runStartedAt: 0, liveActivity: null })
    setAgentChatLog(id, (prev) => {
      const n = [...prev]
      const last = n[n.length - 1]
      if (last?.role === 'assistant') {
        n[n.length - 1] = {
          ...last,
          content: `${(last.content || '').trim() ? `${last.content}\n\n` : ''}⏹ 已终止本次对话`,
          error: true,
        }
      } else {
        n.push({ role: 'assistant', content: '⏹ 已终止本次对话', error: true, time: Date.now() })
      }
      return n
    })
    await loadAgents(id)
  }

  function onKeyDown(e) {
    if (mentionOpen && mentionCandidates.length) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setMentionIdx((i) => (i + 1) % mentionCandidates.length)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setMentionIdx((i) => (i - 1 + mentionCandidates.length) % mentionCandidates.length)
        return
      }
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        insertMention(mentionCandidates[mentionIdx])
        return
      }
      if (e.key === 'Escape') {
        setMentionOpen(false)
        return
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      ask()
    }
  }

  function liveRunSec(a) {
    const local = getAgentSession(a.id)
    if (local.loading && local.runStartedAt) return Math.max(1, Math.round((Date.now() - local.runStartedAt) / 1000))
    if (a.status === 'running' && a.run_started_at) {
      return Math.max(1, Math.round((Date.now() - new Date(a.run_started_at).getTime()) / 1000))
    }
    return 0
  }

  const statusLabel = (a) => {
    const local = getAgentSession(a.id)
    if (local.loading) return '忙碌中'
    return statusZh(a.status)
  }

  const workflowBusy = isWorkflowBusy(selected)
  const liveTask = (tasks || []).find((t) => t.status === 'running' || t.status === 'waiting')
  // 团队占用时不回落到已结束任务；结束后清空进度展示
  const activeTask = liveTask || (workflowBusy ? {
    task_name: selected?.occupancy?.task_name || selected?.occupancy?.source_name || '团队编排任务',
    status: 'running',
    progress: 0,
    message: '团队编排执行中…',
    payload: {},
  } : null)
  const activeProgress = Math.min(100, Math.max(0, activeTask?.progress || 0))
  const activeSteps = activeTask?.payload?.steps || []
  const activeStepIdx = activeTask?.payload?.current_step ?? 0
  const failedStepIdx = activeTask?.payload?.failed ? (activeTask?.payload?.failed_step ?? -1) : -1

  const searchQ = agentSearch.trim().toLowerCase()
  const filteredAgents = useMemo(() => {
    if (!searchQ) return agents
    return agents.filter((a) => (a.name || '').toLowerCase().includes(searchQ))
  }, [agents, searchQ])

  function renderAgentItem(a) {
    const st = statusLabel(a)
    const stClass = statusClass(getAgentSession(a.id).loading ? 'running' : a.status)
    const runSec = liveRunSec(a)
    const pend = (getAgentSession(a.id).permQueue || []).length
    return (
      <div
        key={a.id}
        role="button"
        tabIndex={0}
        className={`ma-item ${selectedId === a.id ? 'active' : ''}`}
        draggable
        onDragStart={(e) => {
          e.dataTransfer.setData('text/agent-id', String(a.id))
          e.dataTransfer.effectAllowed = 'move'
        }}
        onClick={() => setSelectedId(a.id)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            setSelectedId(a.id)
          }
        }}
      >
        <div className="ma-item-head">
          <strong>{a.name}</strong>
          <span className="ma-item-badges">
            {pend > 0 && <span className="admin-badge admin-badge-perm" title="待授权">{pend}</span>}
            <span className={`ma-status st-${stClass}`}>{st}</span>
          </span>
        </div>
        <div className="ma-item-meta">
          <span className="tag engine-tag" style={{ '--eng': engineMeta(a.engine).color }}>{engineMeta(a.engine).label}</span>
          <span>已运行 {fmtAge(a.created_at)}</span>
          {runSec > 0 ? <span className="ma-runlive">本次 {fmtSec(runSec)}</span>
            : a.last_run_ms > 0 ? <span>上次 {fmtSec(Math.round(a.last_run_ms / 1000))}</span> : null}
          {isWorkflowBusy(a) && onOpenWorkflowRun && a.occupancy?.workflow_run_id > 0 && (
            <button
              type="button"
              className="ma-occ-link"
              onClick={(e) => { e.stopPropagation(); onOpenWorkflowRun(a.occupancy.workflow_run_id) }}
            >
              查看团队任务
            </button>
          )}
        </div>
      </div>
    )
  }

  return (
    <section className="ma-panel">
      <div className="ma-toolbar">
        <h2>智能体管理</h2>
        <div className="ma-toolbar-actions">
          <button
            type="button"
            className="ghost"
            onClick={() => { setFolderCreateName('新建文件夹'); setFolderCreateOpen(true) }}
            disabled={busy}
          >
            创建文件夹
          </button>
          <button type="button" className="new" onClick={() => { setNewPolicy(emptyPolicy()); setCreateOpen(true) }}>＋ 一键创建</button>
        </div>
      </div>

      <div className="ma-layout">
        <aside className="ma-list">
          <input
            className="ma-search"
            value={agentSearch}
            onChange={(e) => setAgentSearch(e.target.value)}
            placeholder="搜索智能体名称…"
          />
          {filteredAgents.length === 0 && <p className="empty">暂无匹配的智能体</p>}

          <div
            className="ma-folder-block ma-folder-root"
            onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }}
            onDrop={(e) => {
              e.preventDefault()
              const id = Number(e.dataTransfer.getData('text/agent-id'))
              if (id) moveAgentToFolder(id, 0)
            }}
          >
            <div className="ma-folder-head">
              <div className="ma-folder-title">
                <span className="ma-folder-title-text">未分组</span>
                <span className="ma-folder-title-right">
                  {(() => {
                    const kids = filteredAgents.filter((a) => !a.folder_id)
                    const { direct, wf } = countFolderBusy(kids)
                    return (
                      <>
                        {wf > 0 && <span className="admin-badge admin-badge-workflow ma-folder-badge" title="团队编排忙碌">{wf}</span>}
                        {direct > 0 && <span className="admin-badge ma-folder-badge" title="直接运行中">{direct}</span>}
                        <span className="muted">{kids.length}</span>
                      </>
                    )
                  })()}
                </span>
              </div>
            </div>
            <div className="ma-folder-agents">
              {filteredAgents.filter((a) => !a.folder_id).map(renderAgentItem)}
            </div>
          </div>

          {folders.map((f) => {
            const kids = filteredAgents.filter((a) => Number(a.folder_id) === Number(f.id))
            if (searchQ && kids.length === 0) return null
            const collapsed = !!collapsedFolders[f.id]
            return (
              <div
                key={f.id}
                className="ma-folder-block"
                onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }}
                onDrop={(e) => {
                  e.preventDefault()
                  const id = Number(e.dataTransfer.getData('text/agent-id'))
                  if (id) moveAgentToFolder(id, f.id)
                }}
              >
                <div className="ma-folder-head">
                  <button
                    type="button"
                    className="ma-folder-title"
                    onClick={() => setCollapsedFolders((m) => ({ ...m, [f.id]: !m[f.id] }))}
                  >
                    <span className="ma-folder-title-text">{collapsed ? '▸' : '▾'} {f.name}</span>
                    <span className="ma-folder-title-right">
                      {(() => {
                        const { direct, wf } = countFolderBusy(kids)
                        return (
                          <>
                            {wf > 0 && <span className="admin-badge admin-badge-workflow ma-folder-badge" title="团队编排忙碌">{wf}</span>}
                            {direct > 0 && <span className="admin-badge ma-folder-badge" title="直接运行中">{direct}</span>}
                            <span className="muted">{kids.length}</span>
                          </>
                        )
                      })()}
                    </span>
                  </button>
                  <button
                    type="button"
                    className="ma-folder-edit"
                    title="编辑文件夹"
                    onClick={(e) => {
                      e.stopPropagation()
                      setFolderEdit({ id: f.id, name: f.name })
                      setFolderEditName(f.name || '')
                    }}
                  >
                    编辑
                  </button>
                </div>
                {!collapsed && (
                  <div className="ma-folder-agents">
                    {kids.length === 0
                      ? <p className="ma-folder-empty">拖入智能体到此文件夹</p>
                      : kids.map(renderAgentItem)}
                  </div>
                )}
              </div>
            )
          })}
        </aside>

        <div className="ma-main">
          {!selected ? <p className="empty ma-empty">选择左侧智能体，或点击右上角创建</p> : (
            <div className="ma-detail">
              <header className="ma-detail-head">
                <div className="ma-detail-title">
                  <h3>{selected.name}</h3>
                  <span className="tag engine-tag" style={{ '--eng': engineMeta(selected.engine).color }}>{engineMeta(selected.engine).label}</span>
                  <span className={`ma-status st-${statusClass(getAgentSession(selected.id).loading ? 'running' : selected.status)}`}>
                    {statusLabel(selected)}
                  </span>
                  {workflowBusy && selected.occupancy?.workflow_run_id > 0 && onOpenWorkflowRun && (
                    <button
                      type="button"
                      className="ghost"
                      onClick={() => onOpenWorkflowRun(selected.occupancy.workflow_run_id)}
                    >
                      查看团队任务
                    </button>
                  )}
                  {liveRunSec(selected) > 0 && <span className="ma-runlive">{fmtSec(liveRunSec(selected))}</span>}
                </div>
                <div className="ma-detail-tools">
                  {!workflowBusy && (
                    <label className={`auto-switch ${autoSaving ? 'busy' : ''}`} title="开启后由 AI 审核命令">
                      <input type="checkbox" checked={autoOn} disabled={autoSaving} onChange={(e) => toggleAuto(e.target.checked)} />
                      <span className="slider" />
                      <span className="auto-label">自动审核</span>
                    </label>
                  )}
                  <button type="button" className="ghost" onClick={() => {
                    setRenameDraft(selected.name || '')
                    setRulesDraft(selected.rules_prompt || '')
                    setPolicyDraft(policyFromAgent(selected))
                    setSettingsOpen(true)
                  }}>设置</button>
                  {!workflowBusy && (
                    <>
                      <button type="button" className="ghost" onClick={fetchStatusSnapshot} disabled={loading || compressing}>状态</button>
                      <button type="button" className="ghost" onClick={compressChat} disabled={loading || compressing}>
                        {compressing ? '压缩中…' : '压缩'}
                      </button>
                      <button type="button" className="ghost" onClick={newChat} disabled={loading || compressing}>新对话</button>
                    </>
                  )}
                  <button type="button" className="ghost danger-text" onClick={deleteAgent} disabled={busy || loading}>删除</button>
                </div>
              </header>

              <p className="muted ma-age-line">已运行 {fmtAge(selected.created_at)}</p>

              {!workflowBusy && compressing && <p className="thinking ma-compressing">正在压缩上下文…</p>}

              {!workflowBusy && Object.keys(invokeMap).length > 0 && (
                <div className="ma-invoke-dock">
                  {Object.values(invokeMap).map((inv) => (
                    <div key={inv.calleeId} className={`ma-invoke-chip ${inv.collapsed ? 'collapsed' : ''}`}>
                      <div className="ma-invoke-chip-head">
                        <strong>{inv.name}</strong>
                        <span className={`ma-status st-${statusClass(inv.status)}`}>{statusZh(inv.status)}</span>
                      </div>
                      {!inv.collapsed && (
                        <p className="ma-invoke-chip-body">{inv.message || inv.content?.slice(0, 80) || '执行中…'}</p>
                      )}
                    </div>
                  ))}
                </div>
              )}

              {!workflowBusy && (
              <div className="chatbox ma-chatbox" ref={boxRef} onScroll={onChatScroll}>
                {sess.loadingMore && <p className="thinking">加载更早消息…</p>}
                {sess.hasMore && !sess.loadingMore && (
                  <button type="button" className="ma-load-more" onClick={loadOlder}>加载更早</button>
                )}
                {chatLog.length === 0 && !loading && (
                  <p className="empty-chat">下达任务后，需授权的命令会在此确认</p>
                )}
                {chatLog.map((m, i) => (
                  <div key={m.id || i} className={`msg ${m.role} ${m.decision === 'deny' ? 'cmd-denied' : ''} ${m.role === 'assistant' ? 'ma-flat' : ''}`}>
                    <div className="msg-label">
                      <span>
                        {m.role === 'user' ? '你'
                          : m.role === 'auto' ? '自动审核'
                            : m.role === 'system' ? '系统'
                            : m.role === 'status' ? '状态'
                            : m.role === 'command'
                              ? (m.decision === 'deny' ? '已拒绝命令' : '已执行命令')
                              : m.role === 'invoke_quote' ? `引用 · ${m.calleeName || '智能体'}`
                                : m.role === 'invoke_divider' ? '调用记录'
                                  : selected.name}
                      </span>
                      {m.role === 'command' && m.decidedBy && (
                        <span className="cmd-by">{byLabel(m.decidedBy)}</span>
                      )}
                      {m.role === 'assistant' && (m.content || '').trim() && (
                        <button type="button" className="ma-copy-btn" onClick={() => copyText(m.content)} title="复制回复">
                          复制
                        </button>
                      )}
                      {m.replySec > 0 && <span className="reply-sec">{fmtSec(m.replySec)}</span>}
                    </div>
                    {m.role === 'auto' ? (
                      <div className="auto-trace">{m.note || 'AI 已自动放行'} <code>{m.content}</code></div>
                    ) : m.role === 'command' ? (
                      <div className="ma-timeline-item ma-timeline-cmd">
                        <span className="ma-tl-mark" aria-hidden="true">·</span>
                        <div className="ma-tl-body">
                          <CmdBlock
                            content={m.content}
                            decision={m.decision}
                            risk={m.risk}
                            meaning={m.meaning}
                            note={m.note}
                          />
                        </div>
                      </div>
                    ) : m.role === 'system' ? (
                      <div className="ma-system-note">{m.content}</div>
                    ) : m.role === 'status' ? (
                      <div className="ma-status-snap">
                        <pre>{m.content}</pre>
                      </div>
                    ) : m.role === 'invoke_quote' ? (
                      <details className="ma-quote" open={false}>
                        <summary>{m.calleeName || '被调智能体'} 的结果</summary>
                        <AgentMarkdown content={m.content || ''} agentId={selected.id} authHeaders={authHeaders} />
                      </details>
                    ) : m.role === 'invoke_divider' ? (
                      <div className="ma-divider-msg">{m.content}</div>
                    ) : m.role === 'assistant' ? (
                      <div className="ma-timeline-assistant">
                        <AgentMarkdown content={m.content || ''} agentId={selected.id} authHeaders={authHeaders} />
                      </div>
                    ) : (
                      <div
                        className="msg-body"
                        dangerouslySetInnerHTML={{ __html: escapeHtml(m.content) }}
                      />
                    )}
                  </div>
                ))}
                {permQueue[0] && (
                  <PermCard
                    p={permQueue[0]}
                    more={permQueue.length - 1}
                    busy={permBusy}
                    onDecide={decide}
                  />
                )}
                {loading && (
                  <RunStatus
                    interrupting={interrupting}
                    activity={sess.liveActivity}
                    runSec={liveRunSec(selected)}
                    taskTokens={sess.taskTokens || 0}
                  />
                )}
              </div>
              )}

              {!workflowBusy && (
              <div className="ma-composer">
                <div className="ma-composer-wrap">
                  {mentionOpen && mentionCandidates.length > 0 && (
                    <ul className="ma-mention-list">
                      {mentionCandidates.map((a, i) => (
                        <li key={a.id}>
                          <button
                            type="button"
                            className={i === mentionIdx ? 'active' : ''}
                            onMouseDown={(e) => { e.preventDefault(); insertMention(a) }}
                          >
                            <span className="tag engine-tag" style={{ '--eng': engineMeta(a.engine).color }}>{engineMeta(a.engine).label}</span>
                            {a.name}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                  <textarea
                    ref={composerRef}
                    value={question}
                    onChange={onComposerChange}
                    onKeyDown={onKeyDown}
                    rows={3}
                    placeholder={loading ? '输入插话内容，Enter 发送；@ 可委托其他智能体' : '输入任务，Enter 发送；输入 @ 选择其他智能体'}
                  />
                  <ContextRing used={sess.contextUsed || 0} window={sess.contextWindow || 0} />
                </div>
                <div className="ma-composer-actions">
                  {!loading ? (
                    <button type="button" className="primary" onClick={ask} disabled={!question.trim() || compressing}>
                      发送
                    </button>
                  ) : (
                    <>
                      <button type="button" className="primary" onClick={ask} disabled={!question.trim() || compressing}>
                        {interrupting ? '插话中…' : '插话'}
                      </button>
                      <button type="button" className="stop" onClick={stopRun}>终止</button>
                    </>
                  )}
                </div>
              </div>
              )}

              {workflowBusy && (
                <p className="muted ma-wf-busy-hint">
                  该智能体正在团队编排中执行任务，对话暂不可用；完成后将自动恢复对话窗口。
                  {occupancyLine(selected) ? `（${occupancyLine(selected)}）` : ''}
                </p>
              )}

              <div className="ma-progress-panel">
                <div className="ma-progress-head">
                  <strong>任务进度</strong>
                  {activeTask ? (
                    <span className={`ma-status st-${statusClass(activeTask.status)}`}>{statusZh(activeTask.status)}</span>
                  ) : (
                    <span className="muted">暂无进行中的任务</span>
                  )}
                </div>
                {activeTask && (
                  <>
                    <div className="ma-progress-title">{activeTask.task_name || activeTask.task_id}</div>
                    <div className="agent-progress-bar">
                      <div style={{ width: `${activeProgress}%` }} />
                    </div>
                    <p className="muted ma-progress-msg">{activeTask.message || `${activeProgress}%`}</p>
                    {activeSteps.length > 0 && (
                      <ol className="ma-progress-steps">
                        {activeSteps.map((s, i) => {
                          let cls = ''
                          if (i === failedStepIdx) cls = 'failed'
                          else if (i < activeStepIdx) cls = 'done'
                          else if (i === activeStepIdx) cls = 'current'
                          return (
                            <li key={i} className={cls}>
                              {i === failedStepIdx ? <span className="step-x">✕</span> : null}
                              {s}
                            </li>
                          )
                        })}
                      </ol>
                    )}
                  </>
                )}
              </div>
            </div>
          )}
          {sess.error && <pre className="error">{sess.error}</pre>}
          {selected?.last_error && !loading && <pre className="error">上次错误: {selected.last_error}</pre>}
        </div>
      </div>

      {createOpen && (
        <div className="modal-mask" onClick={() => !busy && setCreateOpen(false)}>
          <form className="modal-card ma-settings" onClick={(e) => e.stopPropagation()} onSubmit={createAgent}>
            <h2>创建智能体</h2>
            <label>名称<input value={newName} onChange={(e) => setNewName(e.target.value)} disabled={busy} /></label>
            <label>
              引擎
              <select value={newEngine} onChange={(e) => setNewEngine(e.target.value)} disabled={busy}>
                {ENGINES.map((e) => <option key={e.id} value={e.id}>{e.label}</option>)}
              </select>
            </label>
            <label>规则描述<textarea rows={4} value={newRules} onChange={(e) => setNewRules(e.target.value)} disabled={busy} /></label>
            <AgentPolicyFields
              value={newPolicy}
              onChange={setNewPolicy}
              workspaces={workspaces}
              disabled={busy}
            />
            <div className="modal-actions">
              <button type="button" disabled={busy} onClick={() => setCreateOpen(false)}>取消</button>
              <button type="submit" disabled={busy}>{busy ? '创建中…' : '创建'}</button>
            </div>
          </form>
        </div>
      )}

      {folderCreateOpen && (
        <div className="modal-mask" onClick={() => !busy && setFolderCreateOpen(false)}>
          <form className="modal-card" onClick={(e) => e.stopPropagation()} onSubmit={createFolder}>
            <h2>创建文件夹</h2>
            <label>
              文件夹名称
              <input
                value={folderCreateName}
                onChange={(e) => setFolderCreateName(e.target.value)}
                disabled={busy}
                placeholder="例如：研发组"
                autoFocus
              />
            </label>
            <div className="modal-actions">
              <button type="button" disabled={busy} onClick={() => setFolderCreateOpen(false)}>取消</button>
              <button type="submit" disabled={busy || !folderCreateName.trim()}>{busy ? '创建中…' : '创建'}</button>
            </div>
          </form>
        </div>
      )}

      {folderEdit && (
        <div className="modal-mask" onClick={() => !busy && setFolderEdit(null)}>
          <form className="modal-card ma-settings" onClick={(e) => e.stopPropagation()} onSubmit={saveFolderEdit}>
            <h2>编辑文件夹</h2>
            <label>
              文件夹名称
              <input
                value={folderEditName}
                onChange={(e) => setFolderEditName(e.target.value)}
                disabled={busy}
                autoFocus
              />
            </label>
            <h3 className="ma-policy-title">文件夹内智能体</h3>
            <div className="ma-folder-agents">
              {agents.filter((a) => Number(a.folder_id) === Number(folderEdit.id)).length === 0 ? (
                <p className="ma-folder-empty">暂无智能体，可从左侧拖入</p>
              ) : (
                agents
                  .filter((a) => Number(a.folder_id) === Number(folderEdit.id))
                  .map((a) => (
                    <div key={a.id} className="ma-folder-member">
                      <span className="ma-folder-member-name">{a.name}</span>
                      <button
                        type="button"
                        className="ghost"
                        disabled={busy}
                        onClick={() => removeAgentFromFolder(a.id)}
                      >
                        移出
                      </button>
                    </div>
                  ))
              )}
            </div>
            <p className="muted">移出后智能体回到「未分组」。</p>
            <div className="modal-actions">
              <button type="button" disabled={busy} onClick={() => setFolderEdit(null)}>取消</button>
              <button type="submit" disabled={busy || !folderEditName.trim()}>{busy ? '保存中…' : '保存名称'}</button>
            </div>
          </form>
        </div>
      )}

      {settingsOpen && selected && (
        <div className="modal-mask" onClick={() => !busy && setSettingsOpen(false)}>
          <form className="modal-card ma-settings" onClick={(e) => e.stopPropagation()} onSubmit={(e) => { e.preventDefault(); saveRules() }}>
            <h2>智能体设置 · {selected.name}</h2>
            <label>
              名称
              <input value={renameDraft} onChange={(e) => setRenameDraft(e.target.value)} disabled={busy} />
            </label>
            <label>
              系统提示词
              <textarea rows={8} value={rulesDraft} onChange={(e) => setRulesDraft(e.target.value)} disabled={busy} />
            </label>
            <AgentPolicyFields
              value={policyDraft}
              onChange={setPolicyDraft}
              workspaces={workspaces}
              disabled={busy}
            />
            <p className="muted">权限会写入系统提示，并每 10 轮重申；工具层会硬拦截违规操作。</p>
            <div className="modal-actions">
              <button type="button" disabled={busy} onClick={() => setSettingsOpen(false)}>取消</button>
              <button type="submit" disabled={busy}>{busy ? '保存中…' : '保存'}</button>
            </div>
          </form>
        </div>
      )}
    </section>
  )
}

function policyFromAgent(a) {
  if (!a) return emptyPolicy()
  const cron = a.schedule_cron || ''
  const preset = SCHEDULE_PRESETS.find((p) => p.cron && p.cron === cron)?.id || ''
  const ws = a.workspace_path || ''
  return {
    allow_write: a.allow_write !== false,
    allow_network: a.allow_network !== false,
    allow_rm: !!a.allow_rm,
    allow_browser: !!a.allow_browser,
    workspace_bound: !!ws,
    workspace_path: ws,
    schedule_enabled: !!a.schedule_enabled,
    schedule_cron: cron,
    schedule_label: a.schedule_label || '',
    schedule_preset: preset,
  }
}

/** 权限 / 工作区 / 定时表单块 */
function AgentPolicyFields({ value, onChange, workspaces, disabled }) {
  const v = value || emptyPolicy()
  function patch(p) {
    onChange({ ...v, ...p })
  }
  return (
    <div className="ma-policy">
      <h3 className="ma-policy-title">权限</h3>
      <CapsuleToggle label="允许写入文件" checked={!!v.allow_write} disabled={disabled} onChange={(c) => patch({ allow_write: c })} />
      <CapsuleToggle label="允许联网搜索" checked={!!v.allow_network} disabled={disabled} onChange={(c) => patch({ allow_network: c })} />
      <CapsuleToggle label="允许执行 rm" checked={!!v.allow_rm} disabled={disabled} onChange={(c) => patch({ allow_rm: c })} />
      <CapsuleToggle
        label="允许操作浏览器"
        checked={!!v.allow_browser}
        disabled={disabled}
        onChange={(c) => patch({ allow_browser: c })}
        hint="若打开此选项，请安装 browser-use 插件。开启后智能体可通过该工具操作浏览器；使用完成网页后须及时关闭，勿留僵尸网页。"
      />

      <h3 className="ma-policy-title">运行边界</h3>
      <CapsuleToggle
        label="限制工作区"
        checked={!!v.workspace_bound}
        disabled={disabled}
        onChange={(c) => patch({ workspace_bound: c, workspace_path: c ? v.workspace_path : '' })}
      />
      {v.workspace_bound && (
        <div className="ma-policy-sub">
          <label>
            选择授权目录
            <select
              value={workspaces.some((w) => w.path === v.workspace_path) ? v.workspace_path : ''}
              disabled={disabled}
              onChange={(e) => patch({ workspace_path: e.target.value })}
            >
              <option value="">请选择…</option>
              {(workspaces || []).map((w) => (
                <option key={w.id || w.path} value={w.path}>{w.name || w.path}</option>
              ))}
            </select>
          </label>
          <label>
            或手动输入绝对路径
            <input
              value={v.workspace_path || ''}
              disabled={disabled}
              placeholder="/Users/.../project"
              onChange={(e) => patch({ workspace_path: e.target.value })}
            />
          </label>
        </div>
      )}

      <CapsuleToggle
        label="开启定时执行"
        checked={!!v.schedule_enabled}
        disabled={disabled}
        onChange={(c) => patch({ schedule_enabled: c })}
      />
      {v.schedule_enabled && (
        <div className="ma-policy-sub ma-schedule">
          <label>
            常用计划
            <select
              value={v.schedule_preset || ''}
              disabled={disabled}
              onChange={(e) => {
                const id = e.target.value
                const p = SCHEDULE_PRESETS.find((x) => x.id === id)
                patch({
                  schedule_preset: id,
                  schedule_cron: p?.cron || v.schedule_cron,
                  schedule_label: p?.label && id ? p.label : v.schedule_label,
                })
              }}
            >
              {SCHEDULE_PRESETS.map((p) => (
                <option key={p.id || 'custom'} value={p.id}>{p.label}</option>
              ))}
            </select>
          </label>
          <label>
            Cron 表达式（分 时 日 月 周）
            <input
              value={v.schedule_cron || ''}
              disabled={disabled}
              placeholder="0 9 * * 1-5"
              onChange={(e) => patch({ schedule_cron: e.target.value, schedule_preset: '' })}
            />
          </label>
          <label>
            计划名称
            <input
              value={v.schedule_label || ''}
              disabled={disabled}
              placeholder="例行巡检"
              onChange={(e) => patch({ schedule_label: e.target.value })}
            />
          </label>
        </div>
      )}
    </div>
  )
}

function CapsuleToggle({ label, checked, disabled, onChange, hint }) {
  return (
    <div className={`ma-capsule-row ${disabled ? 'disabled' : ''}`}>
      <div className="ma-capsule-text">
        <span className="ma-capsule-label">{label}</span>
        {hint && <p className="ma-capsule-hint">{hint}</p>}
      </div>
      <label className={`auto-switch ma-capsule ${disabled ? 'busy' : ''}`}>
        <input type="checkbox" checked={!!checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
        <span className="slider" />
      </label>
    </div>
  )
}

/** 全局授权浮层:面板隐藏/已退到对话页时仍可裁决 */
export function ManagedAgentPermOverlay({ authHeaders, onUnauthorized }) {
  const [, setVer] = useState(0)
  const [busy, setBusy] = useState(false)
  useEffect(() => subscribeAgentSessions(() => setVer((v) => v + 1)), [])
  useEffect(() => {
    const t = setInterval(() => setVer((v) => v + 1), 1000)
    return () => clearInterval(t)
  }, [])
  const pending = allPendingPerms()
  if (pending.length === 0) return null
  const p = pending[0]

  async function decide(behavior) {
    if (busy) return
    setBusy(true)
    try {
      const res = await fetch(`${API}/permissions/decide`, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ request_id: p.request_id, behavior }),
      })
      if (res.status === 401) onUnauthorized?.()
    } finally {
      patchAgentSession(p.agentId, {
        permQueue: (getAgentSession(p.agentId).permQueue || []).filter((x) => x.request_id !== p.request_id),
      })
      setBusy(false)
    }
  }

  return (
    <div className="ma-perm-overlay">
      <div className="ma-perm-overlay-card">
        <div className="perm-title">🔐 智能体 #{p.agentId} 申请授权 · 剩余 {remainSec(p)}s</div>
        <PermCard p={p} more={pending.length - 1} busy={busy} onDecide={(_, b) => decide(b)} />
      </div>
    </div>
  )
}

function PermCard({ p, more, busy, onDecide }) {
  return (
    <div className="perm-card">
      <div className="perm-title">
        🔐 申请使用工具,需要你授权
        <span className={`perm-count ${remainSec(p) <= 10 ? 'urgent' : ''}`}>剩余 {remainSec(p)}s</span>
      </div>
      {p.note && <div className="perm-note">{p.note}</div>}
      <div className="perm-tool">{p.tool_name}</div>
      <CmdBlock content={p.summary} risk={p.risk} meaning={p.meaning} decision="ask" />
      {more > 0 && <div className="perm-more">+{more} 条排队中</div>}
      <div className="perm-actions">
        <button type="button" className="deny" onClick={() => onDecide(p.request_id, 'deny')}>拒绝</button>
        <button type="button" className="allow" onClick={() => onDecide(p.request_id, 'allow')} disabled={busy}>仅本次同意</button>
      </div>
      <div className="perm-hint">授权仅对本次调用生效,超时将自动拒绝。绿/黄/红表示风险：无害 / 需注意 / 高风险。</div>
    </div>
  )
}

/** 命令框：按风险着色 + 底栏含义 + 一键复制；Codex 时间线可附带输出 */
function CmdBlock({ content, risk, meaning, decision, note }) {
  const r = risk || (decision === 'deny' ? 'high' : 'mid')
  const riskText = ({ low: '无害', mid: '需注意', high: '高风险' })[r] || '需注意'
  async function copy() {
    try {
      await navigator.clipboard.writeText(content || '')
    } catch {
      const ta = document.createElement('textarea')
      ta.value = content || ''
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      ta.remove()
    }
  }
  return (
    <div className={`cmd-block risk-${r} ${decision === 'deny' ? 'denied' : ''}`}>
      <div className="cmd-block-main">
        <code>{content}</code>
        <button type="button" className="cmd-copy" onClick={copy} title="复制命令">复制</button>
      </div>
      {note ? <pre className="ma-tl-out">{note}</pre> : null}
      {(meaning || risk) && (
        <div className="cmd-meaning-bar">
          <span className="cmd-risk-dot">{riskText}</span>
          <span>{meaning || `风险等级：${riskText}`}</span>
        </div>
      )}
    </div>
  )
}

function dedupPerm(list) {
  const seen = new Set()
  return list.filter((p) => (seen.has(p.request_id) ? false : (seen.add(p.request_id), true)))
}

function remainSec(p) {
  if (!p?.deadline) return 999
  return Math.max(0, Math.floor((new Date(p.deadline).getTime() - Date.now()) / 1000))
}

function byLabel(by) {
  return ({ user: '人工审批', ai: 'AI 审批', system: '系统自动', timeout: '超时拒绝', disconnect: '断连拒绝', codex: 'Codex' })[by] || by
}

/** 一键复制纯文本 */
async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text || '')
  } catch {
    const ta = document.createElement('textarea')
    ta.value = text || ''
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
  }
}

/** 运行态：银灰 thinking + token + 工具活动条 */
function RunStatus({ interrupting, activity, runSec, taskTokens }) {
  const [phraseIdx, setPhraseIdx] = useState(0)
  useEffect(() => {
    const t = setInterval(() => {
      setPhraseIdx((i) => (i + 1) % THINKING_PHRASES.length)
    }, 2400)
    return () => clearInterval(t)
  }, [])
  const phrase = interrupting ? 'interrupting…' : THINKING_PHRASES[phraseIdx]
  return (
    <div className="ma-run-status">
      <div className="ma-thinking" aria-live="polite">
        <span className="ma-thinking-text">{phrase}</span>
        <span className="ma-thinking-dots" aria-hidden="true">…</span>
        {runSec > 0 && <span className="ma-thinking-sec">{fmtSec(runSec)}</span>}
        {taskTokens > 0 && <span className="ma-thinking-tokens">{fmtToken(taskTokens)} tokens</span>}
      </div>
      {activity?.summary && (
        <div className="ma-activity" title={activity.tool || ''}>
          <span className="ma-activity-dot" />
          <span className="ma-activity-text">{activity.summary}</span>
        </div>
      )}
    </div>
  )
}

/** 输入框右下角真实上下文占比圆环 */
function ContextRing({ used, window: win }) {
  const size = 36
  const stroke = 3.5
  const r = (size - stroke) / 2
  const c = 2 * Math.PI * r
  const pct = win > 0 ? Math.min(1, used / win) : 0
  const dash = c * pct
  const title = win > 0
    ? `上下文 ${fmtToken(used)} / ${fmtToken(win)}（${Math.round(pct * 100)}%）`
    : '上下文用量未知'
  return (
    <div className="ma-ctx-ring" title={title} aria-label={title}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="#334155" strokeWidth={stroke} />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke="#22c55e"
          strokeWidth={stroke}
          strokeDasharray={`${dash} ${c - dash}`}
          strokeLinecap="round"
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
        />
      </svg>
      <span className="ma-ctx-ring-label">{win > 0 ? `${Math.round(pct * 100)}%` : '—'}</span>
    </div>
  )
}

function countFolderBusy(list) {
  let direct = 0
  let wf = 0
  for (const a of list || []) {
    if (isWorkflowBusy(a)) wf += 1
    else if (getAgentSession(a.id).loading || a.status === 'running') direct += 1
  }
  return { direct, wf }
}

function fmtToken(n) {
  n = Number(n) || 0
  if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(Math.round(n))
}

function escapeHtml(s) {
  return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function fmtSec(sec) {
  sec = Math.max(0, Math.round(sec || 0))
  if (sec < 60) return `${sec}s`
  const m = Math.floor(sec / 60)
  const r = sec % 60
  return `${m}m${r ? `${r}s` : ''}`
}

function fmtAge(ts) {
  if (!ts) return '-'
  const sec = Math.floor((Date.now() - new Date(ts).getTime()) / 1000)
  if (sec < 60) return `${Math.max(0, sec)}秒`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}分钟`
  const h = Math.floor(min / 60)
  if (h < 24) return `${h}小时`
  const d = Math.floor(h / 24)
  const rh = h % 24
  return rh ? `${d}天${rh}小时` : `${d}天`
}
