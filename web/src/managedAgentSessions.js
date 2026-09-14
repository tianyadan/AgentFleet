/**
 * 管理型智能体运行态(模块级):切换菜单/登出不丢 SSE / chatLog / 授权队列。
 */
const sessions = new Map()
const listeners = new Set()

function emptySession() {
  return {
    chatLog: [],
    loading: false,
    compressing: false,
    error: '',
    runStartedAt: 0,
    abort: null,
    pendingInterrupt: '',
    interrupting: false,
    permQueue: [],
    autoOn: false,
    totalMessages: 0,
    hasMore: false,
    loadingMore: false,
    convId: 0,
    liveActivity: null,
    taskTokens: 0,
    contextUsed: 0,
    contextWindow: 0,
  }
}

export function getAgentSession(agentId) {
  const id = Number(agentId) || 0
  if (!id) return emptySession()
  if (!sessions.has(id)) sessions.set(id, emptySession())
  return sessions.get(id)
}

export function patchAgentSession(agentId, patch) {
  const id = Number(agentId)
  if (!id) return
  const cur = getAgentSession(id)
  Object.assign(cur, patch)
  listeners.forEach((fn) => fn(id))
}

export function setAgentChatLog(agentId, chatLogOrFn) {
  const s = getAgentSession(agentId)
  const next = typeof chatLogOrFn === 'function' ? chatLogOrFn(s.chatLog) : chatLogOrFn
  patchAgentSession(agentId, { chatLog: next })
}

export function subscribeAgentSessions(fn) {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

export function anyAgentLoading() {
  for (const s of sessions.values()) {
    if (s.loading || s.compressing) return true
  }
  return false
}

/** 所有会话里待裁决的授权(带 agentId) */
export function allPendingPerms() {
  const out = []
  for (const [id, s] of sessions.entries()) {
    for (const p of s.permQueue || []) {
      out.push({ ...p, agentId: id })
    }
  }
  return out
}

export function clearAgentSession(agentId) {
  const id = Number(agentId)
  if (!id) return
  const s = getAgentSession(id)
  s.abort?.abort()
  sessions.set(id, emptySession())
  listeners.forEach((fn) => fn(id))
}

export function listSessionAgentIds() {
  return [...sessions.keys()]
}
