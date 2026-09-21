import { useEffect, useRef, useState } from 'react'
import { marked } from 'marked'
import './App.css'
import { CHANGELOG, CURRENT_VERSION } from './changelog.js'
import ManagedAgentsPanel, { ManagedAgentPermOverlay } from './ManagedAgents.jsx'
import { anyAgentLoading, allPendingPerms, getAgentSession, listSessionAgentIds, subscribeAgentSessions } from './managedAgentSessions.js'
import { WorkflowPanel } from './workflow/index.js'
import WorkflowPermOverlay, { useWorkflowBadges } from './workflow/WorkflowPermOverlay.jsx'
import { AdminMenuIcon } from './adminMenuIcons.jsx'

const API = '/api'
const AUTH_TOKEN_KEY = 'avatar_admin_token'
const CHANGELOG_PAGE_SIZE = 5
marked.setOptions({ breaks: true, gfm: true })

const ADMIN_MENUS = [
  { id: 'history', label: '历史' },
  { id: 'stats', label: '统计' },
  { id: 'agents', label: '数字员工管理' },
  { id: 'plans', label: '项目协作' },
  { id: 'memos', label: '备忘录' },
  { id: 'passwords', label: '常用密码' },
  { id: 'servers', label: '服务器' },
  { id: 'analytics', label: '数据分析' },
  { id: 'changelog', label: '版本' },
]

export default function App() {
  // 登录 / 视图
  const [authToken, setAuthToken] = useState(() => localStorage.getItem(AUTH_TOKEN_KEY) || '')
  const [username, setUsername] = useState('')
  const [view, setView] = useState('chat') // chat | admin
  const [adminMenu, setAdminMenu] = useState('agents')
  const [jumpWorkflowRunId, setJumpWorkflowRunId] = useState(0)
  const [loginOpen, setLoginOpen] = useState(false)
  const [loginUser, setLoginUser] = useState('tianhaowen')
  const [loginPass, setLoginPass] = useState('')
  const [loginErr, setLoginErr] = useState('')
  const [loginBusy, setLoginBusy] = useState(false)
  const [authChecking, setAuthChecking] = useState(!!localStorage.getItem(AUTH_TOKEN_KEY))
  const [clPage, setClPage] = useState(1) // 版本变更记录分页
  const [adminCollapsed, setAdminCollapsed] = useState(false)
  const [runningCount, setRunningCount] = useState(0)
  const [workflowBusyCount, setWorkflowBusyCount] = useState(0)
  const [pendingPermCount, setPendingPermCount] = useState(0)
  const bgTokenRef = useRef('') // 登出后后台任务继续用的 token

  // 模式与会话(单次/长对话各自缓存,切换不串台)
  const [mode, setMode] = useState('single') // single / chat
  const [currentConvId, setCurrentConvId] = useState(0) // 0=未选/单次按次新建
  const [sessions, setSessions] = useState([]) // 列表(按IP分组由后端保证)
  const [compressing, setCompressing] = useState(false)
  const modeCacheRef = useRef({
    single: { convId: 0, chatLog: [] },
    chat: { convId: 0, chatLog: [] },
  })
  const modeRef = useRef(mode)
  modeRef.current = mode

  // 带管理员 JWT 的请求头(登出后若有后台任务则用 bgToken)
  function authHeaders(extra = {}) {
    const h = { ...extra }
    const tok = authToken || bgTokenRef.current
    if (tok) h.Authorization = `Bearer ${tok}`
    return h
  }

  const wfBadges = useWorkflowBadges(authHeaders, !!authToken)

  // 清登录态并回对话页(不中断后台数字员工 SSE)
  function clearAuth(msg) {
    if (anyAgentLoading() && authToken) {
      bgTokenRef.current = authToken
    }
    localStorage.removeItem(AUTH_TOKEN_KEY)
    setAuthToken('')
    setUsername('')
    setView('chat')
    setLoginOpen(false)
    if (msg) setError(msg)
  }

  // 启动时校验本地 token
  useEffect(() => {
    if (!authToken) {
      setAuthChecking(false)
      return
    }
    let cancelled = false
    fetch(`${API}/auth/me`, { headers: { Authorization: `Bearer ${authToken}` } })
      .then(async (r) => {
        if (cancelled) return
        if (!r.ok) {
          clearAuth('')
          return
        }
        const d = await r.json()
        setUsername(d.username || '')
        setView('admin')
      })
      .catch(() => { if (!cancelled) clearAuth('') })
      .finally(() => { if (!cancelled) setAuthChecking(false) })
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function doLogin(e) {
    e?.preventDefault?.()
    if (loginBusy) return
    setLoginBusy(true)
    setLoginErr('')
    try {
      const res = await fetch(`${API}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: loginUser.trim(), password: loginPass }),
      })
      const d = await res.json().catch(() => ({}))
      if (!res.ok) {
        setLoginErr(d.error || '账号或密码错误')
        return
      }
      localStorage.setItem(AUTH_TOKEN_KEY, d.token)
      setAuthToken(d.token)
      bgTokenRef.current = ''
      setUsername(d.username || loginUser.trim())
      setLoginPass('')
      setLoginOpen(false)
      setAdminMenu('agents')
      setView('admin')
      setError('')
    } catch {
      setLoginErr('网络错误，请稍后重试')
    } finally {
      setLoginBusy(false)
    }
  }

  async function doLogout() {
    try {
      await fetch(`${API}/auth/logout`, { method: 'POST', headers: authHeaders() })
    } catch { /* ignore */ }
    clearAuth('')
  }

  // 统计待授权数量；直接运行数由下方 agents 轮询区分团队占用
  useEffect(() => {
    const calc = () => {
      setPendingPermCount(allPendingPerms().length)
    }
    calc()
    return subscribeAgentSessions(calc)
  }, [])

  // 登录后周期性刷新运行中数量徽标（区分直接运行 vs 团队占用）
  useEffect(() => {
    if (!authToken && !bgTokenRef.current) return
    const tick = async () => {
      try {
        const res = await fetch(`${API}/admin/agents`, { headers: authHeaders() })
        if (!res.ok) return
        const d = await res.json()
        const direct = new Set()
        let wf = 0
        ;(d.items || []).forEach((a) => {
          const occ = a.occupancy
          const inWf = occ && (occ.source_type === 'WORKFLOW' || Number(occ.workflow_run_id) > 0)
          if (inWf) {
            wf += 1
            return
          }
          if (a.status === 'running' || getAgentSession(a.id).loading) direct.add(a.id)
        })
        listSessionAgentIds().forEach((id) => {
          if (getAgentSession(id).loading) {
            const item = (d.items || []).find((x) => x.id === id)
            const occ = item?.occupancy
            const inWf = occ && (occ.source_type === 'WORKFLOW' || Number(occ.workflow_run_id) > 0)
            if (!inWf) direct.add(id)
          }
        })
        setRunningCount(direct.size)
        setWorkflowBusyCount(wf)
      } catch { /* ignore */ }
    }
    tick()
    const t = setInterval(tick, 2000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authToken])

  // 提问
  const [question, setQuestion] = useState('')
  const [workspace, setWorkspace] = useState('')
  const [workspaces, setWorkspaces] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [chatLog, setChatLog] = useState([]) // [{role, content, error}]
  const abortRef = useRef(null)
  const [lastFailedQuestion, setLastFailedQuestion] = useState(null)

  // v0.0.5 自动滚动 + 工具授权
  const boxRef = useRef(null)
  const pinnedRef = useRef(true) // 贴底则跟随新内容
  const [permQueue, setPermQueue] = useState([]) // 待裁决的授权请求
  const [permBusy, setPermBusy] = useState(false)
  const [autoOn, setAutoOn] = useState(false) // 本会话 AI 自动审核开关
  const [autoSaving, setAutoSaving] = useState(false)
  const [, setTick] = useState(0) // 倒计时心跳

  // 有挂起授权时每秒心跳;倒计时归零即本地出队(后端 timeout 事件兜底)
  useEffect(() => {
    if (permQueue.length === 0) return
    const t = setInterval(() => {
      setPermQueue((prev) => {
        const alive = prev.filter((p) => remainSec(p) > 0)
        if (alive.length === prev.length) setTick((x) => x + 1)
        return alive
      })
    }, 1000)
    return () => clearInterval(t)
  }, [permQueue.length])

  function remainSec(p) {
    if (!p.deadline) return 999
    return Math.max(0, Math.floor((new Date(p.deadline).getTime() - Date.now()) / 1000))
  }

  // 切换会话时重置 AI 审核显示(后端按会话各自保存)
  useEffect(() => { setAutoOn(false) }, [currentConvId])

  async function ensureConv() {
    if (currentConvId) return currentConvId
    const res = await fetch(`${API}/conversations`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode }),
    })
    const d = await res.json()
    setCurrentConvId(d.id)
    loadSessions()
    return d.id
  }

  async function toggleAuto(v) {
    if (autoSaving) return
    setAutoSaving(true)
    try {
      const conv = v ? await ensureConv() : currentConvId
      if (!conv) { setAutoOn(false); return }
      const res = await fetch(`${API}/permissions/auto`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ conversation_id: conv, enabled: v }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || `自动审核开关失败(${res.status})`)
      }
      setAutoOn(v)
      if (!v) setPermQueue([])
    } catch (e) { setError(String(e)); setAutoOn(false) } finally { setAutoSaving(false) }
  }

  function stop() {
    abortRef.current?.abort()
  }

  // 内容变化时若处于贴底状态则滚到底;新授权请求强制滚到底(阻塞性提示)
  useEffect(() => {
    const el = boxRef.current
    if (el && (pinnedRef.current || permQueue.length > 0)) el.scrollTop = el.scrollHeight
  }, [chatLog, permQueue, loading])

  function onChatScroll() {
    const el = boxRef.current
    if (!el) return
    pinnedRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
  }

  async function decide(requestId, behavior) {
    if (permBusy) return
    setPermBusy(true)
    try {
      await fetch(`${API}/permissions/decide`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ request_id: requestId, behavior }),
      })
    } catch (e) {
      setError(String(e))
    } finally {
      setPermQueue((prev) => prev.filter((p) => p.request_id !== requestId))
      setPermBusy(false)
    }
  }

  // 重连时补齐挂起中的授权(仅限当前会话,避免串到别人的请求)
  useEffect(() => {
    if (view !== 'chat' || !currentConvId) return
    fetch(`${API}/permissions/pending?conversation_id=${currentConvId}`)
      .then((r) => r.json())
      .then((d) => setPermQueue((prev) => dedup([...(d.items || []).filter((p) => p.conversation_id === currentConvId), ...prev])))
      .catch(() => {})
  }, [view, currentConvId])

  function dedup(list) {
    const seen = new Set()
    return list.filter((p) => (seen.has(p.request_id) ? false : (seen.add(p.request_id), true)))
  }

  useEffect(() => {
    fetch(`${API}/workspaces`).then((r) => r.json()).then((d) => setWorkspaces(d.items || []))
      .catch(() => setWorkspaces([]))
  }, [])

  // —— 会话管理 ——
  const [histPage, setHistPage] = useState(1)
  const [histTotal, setHistTotal] = useState(0)
  const histPageSize = 10

  async function loadSessions(page = histPage) {
    if (!authToken) {
      setSessions([])
      setHistTotal(0)
      return
    }
    const p = page || 1
    const res = await fetch(`${API}/conversations?page=${p}&page_size=${histPageSize}`, {
      headers: authHeaders(),
    })
    if (res.status === 401) {
      clearAuth('登录已过期')
      return
    }
    const data = await res.json()
    setSessions(data.items || [])
    setHistPage(data.page || p)
    setHistTotal(data.total || 0)
  }

  async function openConversation(id) {
    // 先把当前模式的对话缓存起来,避免打开历史后串台
    modeCacheRef.current[modeRef.current] = { convId: currentConvId, chatLog }
    const sess = sessions.find((s) => s.id === id)
    const m = sess?.mode === 'chat' ? 'chat' : 'single'
    setMode(m)
    modeRef.current = m
    setCurrentConvId(id)
    setChatLog([])
    setError('')
    setView('chat')
    try {
      const res = await fetch(`${API}/conversations/${id}/messages`, { headers: authHeaders() })
      if (res.status === 401) {
        clearAuth('登录已过期')
        return
      }
      const data = await res.json()
      const log = (data.items || []).map((m0) => ({
        role: m0.role,
        content: m0.content,
        time: m0.created_at ? new Date(m0.created_at).getTime() : 0,
        replySec: m0.reply_ms ? Math.round(m0.reply_ms / 1000) : (m0.role === 'assistant' ? 0 : null),
      }))
      setChatLog(log)
      modeCacheRef.current[m] = { convId: id, chatLog: log }
    } catch (e) { setError(String(e)) }
  }

  // 打开历史会话(历史页按钮)后切回对话页
  async function openConversationFromHistory(id) {
    await openConversation(id)
  }

  async function newConversation() {
    const m = mode === 'chat' ? 'chat' : 'single'
    const res = await fetch(`${API}/conversations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode: m }),
    })
    const data = await res.json()
    setCurrentConvId(data.id)
    setChatLog([])
    setQuestion('')
    setError('')
    modeCacheRef.current[m] = { convId: data.id, chatLog: [] }
    loadSessions()
  }

  // 引擎原生压缩上下文，保留 session，清空平台历史并留系统提示
  async function compressConversation() {
    if (!currentConvId || compressing || loading) return
    setCompressing(true)
    setError('')
    try {
      const res = await fetch(`${API}/conversations/${currentConvId}/compress`, { method: 'POST' })
      const d = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(d.error || '压缩失败')
      await openConversation(currentConvId)
    } catch (e) {
      setError(String(e.message || e))
    } finally {
      setCompressing(false)
    }
  }

  // 切换模式:各自保留会话与消息,互不带走
  function switchMode(m) {
    if (m === mode) return
    modeCacheRef.current[mode] = { convId: currentConvId, chatLog }
    setMode(m)
    modeRef.current = m
    const cached = modeCacheRef.current[m] || { convId: 0, chatLog: [] }
    setChatLog(cached.chatLog || [])
    setCurrentConvId(cached.convId || 0)
    setPermQueue([])
    setError('')
    setQuestion('')
    if (m === 'chat' && !(cached.convId > 0)) {
      fetch(`${API}/conversations`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mode: 'chat' }),
      }).then((r) => r.json()).then((d) => {
        setCurrentConvId(d.id)
        modeCacheRef.current.chat = { convId: d.id, chatLog: modeCacheRef.current.chat?.chatLog || [] }
        loadSessions()
      })
    }
  }

  // —— 提问 ——
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
      try {
        events.push(JSON.parse(line.slice(5).trim()))
      } catch { /* 半包或坏包忽略 */ }
    }
    return { events, rest }
  }

  async function ask() {
    if (!question.trim()) return
    const q = question.trim()
    setQuestion('')
    if (loading) {
      // v0.0.7 插话:先中断当前任务,再带着新问题 + 已产出内容重问
      pendingInterruptRef.current = q
      setInterrupting(true)
      stop()
      return
    }
    await sendQuestion(q)
  }

  // 重试: 重新发送上一条失败的提问
  async function retry() {
    if (!lastFailedQuestion || loading) return
    setChatLog([])
    await sendQuestion(lastFailedQuestion)
  }

  async function sendQuestion(q) {
    if (!q || loading) return
    setLoading(true)
    setError('')
    setLastFailedQuestion(null)
    pinnedRef.current = true // 发送后强制跟随底部

    let convId = currentConvId
    taskStartRef.current = Date.now()
    const userEntry = { role: 'user', content: q, time: Date.now() }
    setChatLog((prev) => [...prev, userEntry])

    let sentChunk = false
    let answer = ''
    let failed = false
    setChatLog((prev) => [...prev, { role: 'assistant', content: '', error: false, time: Date.now(), replySec: 0 }])

    const ac = new AbortController()
    abortRef.current = ac
    try {
      const res = await fetch(`${API}/question`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ question: q, workspace, conversation_id: convId, mode }),
        signal: ac.signal,
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || `HTTP ${res.status}`)
      }
      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''
      let sawDone = false
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const parsed = parseSSE(buf)
        buf = parsed.rest
        for (const ev of parsed.events) {
          if (ev.type === 'chunk') {
            sentChunk = true
            const piece = (ev.content || '').replace(/\\n/g, '\n')
            answer += piece
            setChatLog((prev) => {
              const n = [...prev]
              if (n.length && n[n.length - 1].role === 'assistant') {
                const last = n[n.length - 1]
                n[n.length - 1] = { role: 'assistant', content: (last.content || '') + piece, error: false }
              } else {
                n.push({ role: 'assistant', content: piece, error: false })
              }
              return n
            })
          } else if (ev.type === 'rejected') {
            failed = true
            setChatLog((prev) => {
              const n = [...prev]
              n[n.length - 1] = { role: 'assistant', content: `⚠️ ${ev.reason || '工作区未授权'}`, error: true }
              return n
            })
          } else if (ev.type === 'error') {
            // 已有部分输出时保留正文,仅追加错误提示,避免像「莫名中断」
            failed = !sentChunk
            setChatLog((prev) => {
              const n = [...prev]
              const base = answer || n[n.length - 1].content || ''
              n[n.length - 1] = {
                role: 'assistant',
                content: sentChunk ? `${base}\n\n❌ ${ev.message || '出错了'}` : `❌ ${ev.message || '出错了'}`,
                error: true,
              }
              return n
            })
          } else if (ev.type === 'permission_auto') {
            setChatLog((prev) => [...prev, { role: 'auto', content: `${ev.tool_name} · ${(ev.summary || '').slice(0, 80)}`, note: ev.note }])
          } else if (ev.type === 'command') {
            // v0.0.9: 已执行命令直接显示在对话流,便于审阅
            setChatLog((prev) => [...prev, { role: 'command', content: ev.summary || ev.tool_name }])
          } else if (ev.type === 'context_compacted') {
            // 保留本轮 user + assistant，避免 Codex 压缩同步把刚出现的回复清掉
            setChatLog((prev) => {
              let lastUser = null
              let lastAsst = null
              let userIdx = -1
              for (let i = prev.length - 1; i >= 0; i--) {
                if (prev[i].role === 'user' && (prev[i].content || '').trim()) {
                  userIdx = i
                  lastUser = { role: 'user', content: prev[i].content }
                  break
                }
              }
              if (userIdx >= 0) {
                for (let i = prev.length - 1; i > userIdx; i--) {
                  if (prev[i].role === 'assistant' && (prev[i].content || '').trim()) {
                    lastAsst = { role: 'assistant', content: prev[i].content, replySec: prev[i].replySec }
                    break
                  }
                }
              }
              const next = [{ role: 'assistant', content: ev.content || '📦 上下文已由引擎压缩，会话已保留' }]
              if (lastUser) next.push(lastUser)
              if (lastAsst) next.push(lastAsst)
              return next
            })
          } else if (ev.type === 'permission_request') {
            setPermQueue((prev) => dedup([...prev, ev]))
          } else if (ev.type === 'permission_resolved') {
            // 后端已裁决(超时/断连/他人已点),本地同步出队
            setPermQueue((prev) => prev.filter((p) => p.request_id !== ev.request_id))
          } else if (ev.type === 'done') {
            sawDone = true
            if (ev.conversation_id) setCurrentConvId(ev.conversation_id)
            if (ev.reply_ms) {
              setTaskSec(Math.max(1, Math.round(ev.reply_ms / 1000)))
              setChatLog((prev) => {
                const n = [...prev]
                // 回写耗时到最近一条 assistant(命令可能插在末尾)
                for (let i = n.length - 1; i >= 0; i--) {
                  if (n[i].role === 'assistant') {
                    n[i] = { ...n[i], replySec: Math.round(ev.reply_ms / 1000) }
                    break
                  }
                }
                return n
              })
            }
            // 单次模式:一轮结束后加分割线提示
            if (modeRef.current === 'single') {
              setChatLog((prev) => [
                ...prev,
                { role: 'divider', content: '本轮询问已结束 · 可继续输入开启新一轮' },
              ])
            }
            loadSessions()
          }
        }
      }
      if (!sentChunk && !sawDone) {
        // 无任何输出就结束(异常终止)
        failed = true
        setChatLog((prev) => {
          const n = [...prev]
          n[n.length - 1] = { role: 'assistant', content: n[n.length - 1].content || '⚠️ 请求中断,无返回结果', error: true }
          return n
        })
      } else if (sentChunk && !sawDone) {
        // 流被代理/网络掐断:保留已输出,标记未完整结束
        setChatLog((prev) => {
          const n = [...prev]
          const last = n[n.length - 1]
          if (last && !String(last.content || '').includes('连接中断')) {
            n[n.length - 1] = { ...last, content: `${last.content || ''}\n\n⚠️ 连接中断,回答可能不完整`, error: false }
          }
          return n
        })
      }
    } catch (e) {
      if (e && e.name === 'AbortError') {
        // 用户主动终止或打断:保留已产出内容,标记已停止,不进入重试
        const isInterrupt = !!pendingInterruptRef.current
        setChatLog((prev) => {
          const n = [...prev]
          const last = n[n.length - 1]
          n[n.length - 1] = { role: 'assistant', content: (last.content || '') + (isInterrupt ? '' : '\n\n⏹ 已停止回答'), error: false }
          return n
        })
        if (isInterrupt) {
          // 打断时不 return,继续走下方 finally → 触发 pendingInterruptRef 重问
        } else {
          return
        }
      } else {
        failed = true
        setError(String(e))
        setChatLog((prev) => {
          const n = [...prev]
          n[n.length - 1] = { role: 'assistant', content: `❌ ${String(e)}`, error: true }
          return n
        })
      }
    } finally {
      setLoading(false)
      abortRef.current = null
      if (failed) setLastFailedQuestion(q)
      loadSessions()
    }
    // 若存在打断请求,自动重问(带上之前 aborted 的部分答案,claude 会读到已进展)
    if (pendingInterruptRef.current) {
      const nq = pendingInterruptRef.current
      pendingInterruptRef.current = ''
      setInterrupting(false)
      await sendQuestion(nq)
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      ask()
    }
  }

  // —— 管理菜单 ——
  function switchAdminMenu(id) {
    setAdminMenu(id)
    if (id === 'history') loadSessions(1)
    if (id === 'stats') loadStats()
    if (id === 'changelog') setClPage(1)
  }

  async function loadStats() {
    if (!authToken) return
    const res = await fetch(`${API}/stats/days`, { headers: authHeaders() })
    if (res.status === 401) {
      clearAuth('登录已过期')
      return
    }
    const data = await res.json()
    setStats(data.items || [])
    setOpenStatDay({})
  }
  async function loadStatDay(date) {
    const res = await fetch(`${API}/stats/days/${encodeURIComponent(date)}`, { headers: authHeaders() })
    if (res.status === 401) {
      clearAuth('登录已过期')
      return
    }
    const data = await res.json()
    setOpenStatDay((prev) => ({ ...prev, [date]: data.items || [] }))
  }
  const [stats, setStats] = useState([])
  const [openStatDay, setOpenStatDay] = useState({}) // date -> IP rows

  async function copyText(t) {
    try { await navigator.clipboard.writeText(t) } catch { /* ignore */ }
  }

  // v0.0.7 开场白(邀请语不足 200 字)
  const OPENING = "👋 我是 **E-bot 数字员工**,田浩文的打杂分身,帮你 👉 对接接口 \u2022 查配置 \u2022 查数据 \u2022 查提交 \u2022 翻本地文档 \u2022 梳理业务逻辑。\n有啥代码/数据和「跑腿」的活儿,直接丢给我 💻🔌📦。当然,我只**查**,不改 😉。"

  // 任务总耗时计时(从提问发起到 done)
  const taskStartRef = useRef(0)
  const [taskSec, setTaskSec] = useState(0)

  // 打印任务秒数
  useEffect(() => {
    if (!loading) { setTaskSec(0); return }
    const t = setInterval(() => setTaskSec((x) => x + 1), 1000)
    return () => clearInterval(t)
  }, [loading])

  // 打断:运行中输入新问题
  const pendingInterruptRef = useRef('')
  const [interrupting, setInterrupting] = useState(false)

  // 进入管理台历史菜单时加载列表
  useEffect(() => {
    if (view === 'admin' && adminMenu === 'history' && authToken) loadSessions(1)
    if (view === 'admin' && adminMenu === 'stats' && authToken) loadStats()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view, adminMenu, authToken])

  if (authChecking) {
    return <div className="app"><p className="empty">校验登录中…</p></div>
  }

  // 管理台布局
  if (view === 'admin') {
    return (
      <div className="app app-admin">
        <header className="topbar admin-topbar">
          <h1>
            🤖 E-bot 管理台
            {username ? <span className="admin-user">@{username}</span> : null}
            <span className="admin-ver">{CURRENT_VERSION}</span>
          </h1>
          <div className="topbar-actions">
            <button type="button" onClick={() => setView('chat')}>返回对话</button>
            <button type="button" className="danger" onClick={doLogout}>退出</button>
          </div>
        </header>
        <div className={`admin-layout ${adminCollapsed ? 'collapsed' : ''}`}>
          <aside className="admin-side">
            <button
              type="button"
              className="admin-collapse-btn"
              onClick={() => setAdminCollapsed((v) => !v)}
              title={adminCollapsed ? '展开菜单' : '折叠菜单'}
            >
              {adminCollapsed ? '»' : '« 折叠'}
            </button>
            {ADMIN_MENUS.map((m) => (
              <button
                key={m.id}
                type="button"
                className={adminMenu === m.id ? 'active' : ''}
                onClick={() => switchAdminMenu(m.id)}
                title={m.label}
              >
                <span className="admin-side-main">
                  <AdminMenuIcon name={m.id} />
                  <span className="admin-side-label">{m.label}</span>
                </span>
                <span className="admin-side-badges">
                  {m.id === 'agents' && pendingPermCount > 0 && (
                    <span className="admin-badge admin-badge-perm" title="待授权命令">{pendingPermCount}</span>
                  )}
                  {m.id === 'agents' && workflowBusyCount > 0 && (
                    <span className="admin-badge admin-badge-workflow" title="项目协作忙碌">{workflowBusyCount}</span>
                  )}
                  {m.id === 'agents' && runningCount > 0 && (
                    <span className="admin-badge" title="直接运行中">{runningCount}</span>
                  )}
                  {m.id === 'plans' && wfBadges.pending > 0 && (
                    <span className="admin-badge admin-badge-perm" title="团队待授权命令">{wfBadges.pending}</span>
                  )}
                  {m.id === 'plans' && wfBadges.running > 0 && (
                    <span className="admin-badge" title="运行中团队">{wfBadges.running}</span>
                  )}
                </span>
              </button>
            ))}
          </aside>
          <main className="admin-main">
            {adminMenu === 'history' && (
              <section className="list">
                <p className="empty-hint">对话历史 · 每页 {histPageSize} 条 · 共 {histTotal} 条</p>
                {sessions.length === 0 && <p className="empty">暂无对话记录</p>}
                {sessions.map((s) => (
                  <button key={s.id} type="button" className="card session" onClick={() => openConversationFromHistory(s.id)}>
                    <div className="card-head">
                      <span className="tag">{s.mode === 'chat' ? '长对话' : '单次'}</span>
                      <span className="tag ip-tag">{s.user_ip}</span>
                      <span>{new Date(s.updated_at).toLocaleString()}</span>
                    </div>
                    <p className="q">{s.preview || '(空对话)'}</p>
                  </button>
                ))}
                <div className="pager">
                  <button type="button" disabled={histPage <= 1} onClick={() => loadSessions(histPage - 1)}>上一页</button>
                  <span>第 {histPage} / {Math.max(1, Math.ceil(histTotal / histPageSize) || 1)} 页</span>
                  <button type="button" disabled={histPage * histPageSize >= histTotal} onClick={() => loadSessions(histPage + 1)}>下一页</button>
                </div>
              </section>
            )}
            {adminMenu === 'stats' && (
              <section className="list">
                <p className="empty-hint">按日期分组(最新在前)· 点击展开查看各 IP 成功/失败次数</p>
                {stats.length === 0 && <p className="empty">暂无统计数据</p>}
                {stats.map((s) => (
                  <div key={s.date} className="ip-group">
                    <button type="button" className="ip-folder" onClick={() => {
                      const open = openStatDay[s.date]
                      if (open) {
                        setOpenStatDay((prev) => { const n = { ...prev }; delete n[s.date]; return n })
                      } else {
                        loadStatDay(s.date)
                      }
                    }}>
                      <span className="ip-folder-caret">{openStatDay[s.date] ? '▾' : '▸'}</span>
                      <span className="ip-folder-name">{String(s.date).slice(0, 10)}</span>
                      <span className="ip-folder-count">成功 {s.ok_count} · 失败 {s.fail_count} · 共 {s.total}</span>
                    </button>
                    {openStatDay[s.date] && (
                      <div className="ip-items">
                        <table>
                          <thead><tr><th>IP</th><th>成功</th><th>失败</th><th>合计</th></tr></thead>
                          <tbody>
                            {openStatDay[s.date].map((row, i) => (
                              <tr key={i}><td>{row.user_ip}</td><td>{row.ok_count}</td><td>{row.fail_count}</td><td>{row.total}</td></tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>
                ))}
              </section>
            )}
            {/* 切菜单只隐藏,避免中断后台 SSE */}
            <div style={{ display: adminMenu === 'agents' ? 'block' : 'none' }}>
              <ManagedAgentsPanel
                authHeaders={authHeaders}
                onUnauthorized={() => clearAuth('登录已过期')}
                active={adminMenu === 'agents'}
                onOpenWorkflowRun={(runId) => {
                  setJumpWorkflowRunId(runId)
                  setAdminMenu('plans')
                }}
              />
            </div>
            <div style={{ display: adminMenu === 'plans' ? 'block' : 'none' }}>
              <WorkflowPanel
                authHeaders={authHeaders}
                onUnauthorized={() => clearAuth('登录已过期')}
                jumpRunId={jumpWorkflowRunId}
                onJumpConsumed={() => setJumpWorkflowRunId(0)}
              />
            </div>
            {adminMenu === 'changelog' && (() => {
              const total = CHANGELOG.length
              const pages = Math.max(1, Math.ceil(total / CHANGELOG_PAGE_SIZE))
              const page = Math.min(clPage, pages)
              const slice = CHANGELOG.slice((page - 1) * CHANGELOG_PAGE_SIZE, page * CHANGELOG_PAGE_SIZE)
              return (
                <section className="list changelog">
                  <p className="empty-hint">版本变更记录 · 当前 {CURRENT_VERSION} · 每页 {CHANGELOG_PAGE_SIZE} 条 · 共 {total} 个版本</p>
                  {slice.map((v) => (
                    <article key={v.version} className="version-card">
                      <div className="version-head">
                        <span className="version-tag">{v.version}</span>
                        <span className="version-date">{v.date}</span>
                      </div>
                      <ul>
                        {(v.items || []).map((it, i) => <li key={i}>{it}</li>)}
                      </ul>
                    </article>
                  ))}
                  <div className="pager">
                    <button type="button" disabled={page <= 1} onClick={() => setClPage(page - 1)}>上一页</button>
                    <span>第 {page} / {pages} 页</span>
                    <button type="button" disabled={page >= pages} onClick={() => setClPage(page + 1)}>下一页</button>
                  </div>
                </section>
              )
            })()}
            {!['history', 'stats', 'agents', 'plans', 'changelog'].includes(adminMenu) && (
              <section className="admin-placeholder">
                <h2>{ADMIN_MENUS.find((m) => m.id === adminMenu)?.label || ''}</h2>
                <p>即将开放</p>
              </section>
            )}
            {error && <pre className="error">{error}</pre>}
          </main>
        </div>
        {adminMenu !== 'agents' && (
          <ManagedAgentPermOverlay
            authHeaders={authHeaders}
            onUnauthorized={() => clearAuth('登录已过期')}
          />
        )}
        {authToken && (
          <WorkflowPermOverlay
            authHeaders={authHeaders}
            onUnauthorized={() => clearAuth('登录已过期')}
          />
        )}
      </div>
    )
  }

  return (
    <div className="app">
      <header className="topbar">
        <h1>🤖 E-bot</h1>
        <nav className="topbar-nav">
          <span className="brand-slogan">多智协同，分身执行</span>
          <div className="topbar-actions">
            {authToken ? (
              <button type="button" onClick={() => { setView('admin'); setAdminMenu('agents') }}>管理台</button>
            ) : (
              <button type="button" onClick={() => { setLoginErr(''); setLoginOpen(true) }}>登录</button>
            )}
          </div>
        </nav>
      </header>

      {loginOpen && (
        <div className="modal-mask" onClick={() => !loginBusy && setLoginOpen(false)}>
          <form className="modal-card" onClick={(e) => e.stopPropagation()} onSubmit={doLogin}>
            <h2>管理员登录</h2>
            <label>
              账号
              <input value={loginUser} onChange={(e) => setLoginUser(e.target.value)} autoComplete="username" disabled={loginBusy} />
            </label>
            <label>
              密码
              <input type="password" value={loginPass} onChange={(e) => setLoginPass(e.target.value)} autoComplete="current-password" disabled={loginBusy} />
            </label>
            {loginErr && <p className="login-err">{loginErr}</p>}
            <div className="modal-actions">
              <button type="button" disabled={loginBusy} onClick={() => setLoginOpen(false)}>取消</button>
              <button type="submit" disabled={loginBusy}>{loginBusy ? '登录中…' : '登录'}</button>
            </div>
          </form>
        </div>
      )}

      <section className="ask">
          {/* 模式切换 + 新建对话 */}
          <div className="modebar">
            <div className="modes">
              <button className={mode === 'single' ? 'active' : ''} onClick={() => switchMode('single')}>单次询问</button>
              <button className={mode === 'chat' ? 'active' : ''} onClick={() => switchMode('chat')}>长对话</button>
            </div>
            <div className="modebar-right">
              <label className={`auto-switch ${autoSaving ? 'busy' : ''}`} title="开启后由 AI 审核权限,只有拿不准才弹窗">
                <input type="checkbox" checked={autoOn} disabled={autoSaving} onChange={(e) => toggleAuto(e.target.checked)} />
                <span className="slider" />
                <span className="auto-label">AI 自动审核</span>
              </label>
              {mode === 'chat' && (
                <>
                  <button
                    className="new"
                    type="button"
                    disabled={!currentConvId || compressing || loading}
                    onClick={compressConversation}
                    title="调用引擎原生压缩，保留会话"
                  >
                    {compressing ? '压缩中…' : '压缩'}
                  </button>
                  <button className="new" onClick={newConversation}>＋ 新建对话</button>
                </>
              )}
            </div>
          </div>

          {/* 当前长对话提示 */}
          {mode === 'chat' && (
            <div className="conv-badge">
              {currentConvId ? `当前会话 #${currentConvId} · 引擎续聊` : '请点击右上角「新建对话」开始'}
            </div>
          )}

          {/* 聊天区 */}
          <div className="chatbox" ref={boxRef} onScroll={onChatScroll}>
            {chatLog.length === 0 && (
              <div className="opening" dangerouslySetInnerHTML={{ __html: marked.parse(OPENING) }} />
            )}
            {chatLog.map((m, i) => (
              m.role === 'divider' ? (
                <div key={i} className="turn-divider" role="separator">
                  <span>{m.content || '本轮询问已结束'}</span>
                </div>
              ) : (
              <div key={i} className={`msg ${m.role}`}>
                <div className="msg-label">
                  <span>{m.role === 'user' ? '你' : (m.role === 'auto' ? '自动审核' : (m.role === 'command' ? '已执行命令' : 'E-bot'))}</span>
                  <span className="msg-time">{m.time ? new Date(m.time).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : ''}</span>
                  {m.replySec > 0 && <span className="reply-sec">回复 {fmtSec(m.replySec)}</span>}
                </div>
                {m.role === 'auto' ? (
                  <div className="auto-trace">🤖 {m.note || 'AI 已自动放行'} <code>{m.content}</code></div>
                ) : m.role === 'command' ? (
                  <div className="cmd-msg"><code>{m.content}</code><button className="cmd-copy" onClick={() => copyText(m.content)}>复制</button></div>
                ) : (
                  <div className="msg-body"
                    dangerouslySetInnerHTML={{ __html: m.role === 'user' ? escapeHtml(m.content) : marked.parse(m.content) }}
                  />
                )}
              </div>
              )
            ))}
            {loading && (
              <p className="thinking">{interrupting ? `⏳ 打断中… ${taskSec}s` : `思考中… ${taskSec}s(可输入新消息打断)`}</p>
            )}
            {permQueue[0] && (
              <div className="perm-card">
                <div className="perm-title">
                  🔐 E-bot 申请使用工具,需要你授权
                  <span className={`perm-count ${remainSec(permQueue[0]) <= 10 ? 'urgent' : ''}`}>
                    剩余 {remainSec(permQueue[0])}s
                  </span>
                </div>
                {permQueue[0].note && <div className="perm-note">{permQueue[0].note}</div>}
                <div className="perm-tool">{permQueue[0].tool_name}</div>
                <pre className="perm-summary">{permQueue[0].summary}</pre>
                {(permQueue[0].meaning || permQueue[0].risk) && (
                  <div className="perm-review">
                    {permQueue[0].meaning && <div className="perm-meaning">💬 含义：{permQueue[0].meaning}</div>}
                    {permQueue[0].risk && (
                      <div className={`perm-risk risk-${permQueue[0].risk}`}>
                        ⚠️ 风险：<b>{riskLabel(permQueue[0].risk)}</b>
                      </div>
                    )}
                  </div>
                )}
                {permQueue.length > 1 && <div className="perm-more">+{permQueue.length - 1} 条排队中</div>}
                <div className="perm-actions">
                  <button className="deny" onClick={() => decide(permQueue[0].request_id, 'deny')}>拒绝</button>
                  <button className="allow" onClick={() => decide(permQueue[0].request_id, 'allow')} disabled={permBusy}>
                    仅本次同意
                  </button>
                </div>
                <div className="perm-hint">授权仅对本次调用生效,超时将自动拒绝。</div>
              </div>
            )}
          </div>

          <textarea
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder={mode === 'chat' ? '继续提问(Enter 发送, Shift+Enter 换行)' : '例如：kfi-cloud-api 里的订单公式？(Enter 发送)'}
            rows={3}
          />
          <div className="row">
            <select value={workspace} onChange={(e) => setWorkspace(e.target.value)}>
              <option value="">全部授权工作区</option>
              {workspaces.map((w) => (
                <option key={w.id} value={w.path}>{w.name}</option>
              ))}
            </select>
            {loading && (
              <button className="stop" onClick={stop} disabled={!loading}>■ 终止</button>
            )}
            <button onClick={ask}>{loading ? '↪ 插话' : '发送'}</button>
            {lastFailedQuestion && !loading && (
              <button className="retry" onClick={retry}>↻ 重试</button>
            )}
          </div>
          {error && <pre className="error">{error}</pre>}
        </section>

      <footer className="app-footer">当前版本 {CURRENT_VERSION}</footer>
      {/* 登出后仍挂载隐藏面板,保持订阅;授权浮层可在对话页裁决 */}
      <div style={{ display: 'none' }} aria-hidden>
        <ManagedAgentsPanel
          authHeaders={authHeaders}
          onUnauthorized={() => clearAuth('登录已过期')}
          active={false}
        />
      </div>
      <ManagedAgentPermOverlay
        authHeaders={authHeaders}
        onUnauthorized={() => clearAuth('登录已过期')}
      />
    </div>
  )
}

function fmtSec(sec) {
  sec = Math.max(0, Math.round(sec || 0))
  if (sec < 60) return `${sec}s`
  const m = Math.floor(sec / 60), r = sec % 60
  return `${m}m${r ? r + 's' : ''}`
}


function escapeHtml(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}


// riskLabel 把英文风险等级(map) 到中文
function riskLabel(r) {
  const map = { low: '低', mid: '中', high: '高' }
  return map[r] || r
}
