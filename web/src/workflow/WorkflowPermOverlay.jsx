import { useEffect, useState } from 'react'
import { PermissionCard } from '../PermissionCard.jsx'

const API = '/api'

/**
 * 项目协作全局授权浮层：即使不在运行详情页也能裁决。
 */
export default function WorkflowPermOverlay({ authHeaders, onUnauthorized }) {
  const [items, setItems] = useState([])
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let stop = false
    async function pull() {
      try {
        const r = await fetch(`${API}/admin/workflows/permission-pending`, { headers: authHeaders() })
        if (r.status === 401) { onUnauthorized?.(); return }
        if (!r.ok) return
        const d = await r.json()
        if (!stop) setItems(Array.isArray(d.items) ? d.items : [])
      } catch { /* ignore */ }
    }
    pull()
    const t = setInterval(pull, 1500)
    return () => { stop = true; clearInterval(t) }
  }, [authHeaders, onUnauthorized])

  if (items.length === 0) return null
  const p = items[0]

  async function decide(requestId, behavior) {
    if (busy) return
    setBusy(true)
    try {
      const res = await fetch(`${API}/permissions/decide`, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ request_id: requestId, behavior }),
      })
      if (res.status === 401) onUnauthorized?.()
      setItems((prev) => prev.filter((x) => x.request_id !== requestId))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="ma-perm-overlay wf-perm-overlay">
      <div className="ma-perm-overlay-card">
        <div className="perm-title">🔐 项目协作 · 命令待授权</div>
        <PermissionCard
          p={p}
          more={items.length - 1}
          busy={busy}
          onDecide={decide}
          title="协作节点申请执行命令"
        />
      </div>
    </div>
  )
}

/** 轮询项目协作待授权数与运行中团队数 */
export function useWorkflowBadges(authHeaders, enabled) {
  const [pending, setPending] = useState(0)
  const [running, setRunning] = useState(0)
  useEffect(() => {
    if (!enabled) return undefined
    let stop = false
    async function tick() {
      try {
        const r = await fetch(`${API}/admin/workflows/permission-pending`, { headers: authHeaders() })
        if (!r.ok) return
        const d = await r.json()
        if (stop) return
        setPending(Array.isArray(d.items) ? d.items.length : 0)
        setRunning(Number(d.running_teams) || 0)
      } catch { /* ignore */ }
    }
    tick()
    const t = setInterval(tick, 2000)
    return () => { stop = true; clearInterval(t) }
  }, [authHeaders, enabled])
  return { pending, running }
}
