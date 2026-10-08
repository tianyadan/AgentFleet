import { useEffect, useMemo, useState } from 'react'
import { agentAvatarUrl } from './agentAvatar.js'

const API = '/api'
const PAGE_SIZE = 12 // 一页 12 张：4 列 × 3 行

/** 设置图标（齿轮，避免射线状「太阳」观感） */
function GearIcon() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  )
}

/** 删除图标 */
function TrashIcon() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M3 6h18" />
      <path d="M8 6V4h8v2" />
      <path d="M19 6l-1 14H6L5 6" />
      <path d="M10 11v6M14 11v6" />
    </svg>
  )
}

/**
 * 数字人管理：卡片网格、前台助理、多选解聘、分页（12/页）。
 * onOpenSettings(agent) / onHire 由父级打开现有招聘/设置流程。
 */
export default function DigitalHumanCards({ authHeaders, onUnauthorized, onOpenSettings, onHire, active }) {
  const [agents, setAgents] = useState([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState(() => new Set())
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [page, setPage] = useState(1)

  const totalPages = Math.max(1, Math.ceil(agents.length / PAGE_SIZE))
  const pageItems = useMemo(() => {
    const p = Math.min(page, totalPages)
    const start = (p - 1) * PAGE_SIZE
    return agents.slice(start, start + PAGE_SIZE)
  }, [agents, page, totalPages])

  async function load() {
    setLoading(true)
    setErr('')
    try {
      const res = await fetch(`${API}/admin/agents`, { headers: authHeaders() })
      if (res.status === 401) { onUnauthorized?.(); return }
      const d = await res.json()
      const list = Array.isArray(d) ? d : (d.items || [])
      setAgents(list)
      setPage((p) => {
        const pages = Math.max(1, Math.ceil(list.length / PAGE_SIZE))
        return Math.min(p, pages)
      })
    } catch (e) {
      setErr(String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (active) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active])

  function toggleSelect(id) {
    setSelected((prev) => {
      const n = new Set(prev)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  }

  // 全选当前页
  function toggleAll() {
    const ids = pageItems.map((a) => a.id)
    const allOn = ids.length > 0 && ids.every((id) => selected.has(id))
    setSelected((prev) => {
      const n = new Set(prev)
      if (allOn) ids.forEach((id) => n.delete(id))
      else ids.forEach((id) => n.add(id))
      return n
    })
  }

  const pageAllSelected = pageItems.length > 0 && pageItems.every((a) => selected.has(a.id))

  async function deleteOne(a) {
    if (!window.confirm(`确认解聘「${a.name}」？`)) return
    setBusy(true)
    try {
      const res = await fetch(`${API}/admin/agents/${a.id}`, { method: 'DELETE', headers: authHeaders() })
      if (res.status === 401) { onUnauthorized?.(); return }
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        setErr(d.error || '解聘失败')
        return
      }
      setSelected((prev) => { const n = new Set(prev); n.delete(a.id); return n })
      await load()
    } finally {
      setBusy(false)
    }
  }

  async function deleteSelected() {
    const ids = [...selected]
    if (!ids.length) return
    if (!window.confirm(`确认解聘选中的 ${ids.length} 位数字人？`)) return
    setBusy(true)
    try {
      const res = await fetch(`${API}/admin/agents/batch-delete`, {
        method: 'POST',
        headers: { ...authHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids }),
      })
      if (res.status === 401) { onUnauthorized?.(); return }
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        setErr(d.error || '批量解聘失败')
        return
      }
      setSelected(new Set())
      await load()
    } finally {
      setBusy(false)
    }
  }

  async function toggleReceptionist(a) {
    setBusy(true)
    setErr('')
    try {
      const url = `${API}/admin/agents/${a.id}/receptionist`
      const res = await fetch(url, {
        method: a.is_receptionist ? 'DELETE' : 'POST',
        headers: authHeaders(),
      })
      if (res.status === 401) { onUnauthorized?.(); return }
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        setErr(d.error || '设置失败')
        return
      }
      await load()
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="dh-manage">
      <div className="dh-toolbar">
        <h2>数字人管理</h2>
        <div className="dh-toolbar-actions">
          <label className="dh-check-all">
            <input type="checkbox" checked={pageAllSelected} onChange={toggleAll} disabled={!pageItems.length || busy} />
            全选本页
          </label>
          <button type="button" className="ghost" disabled={!selected.size || busy} onClick={deleteSelected}>
            解聘所选{selected.size ? ` (${selected.size})` : ''}
          </button>
          <button type="button" className="new" disabled={busy} onClick={() => onHire?.()}>＋ 一键招聘</button>
          <button type="button" className="ghost" disabled={loading || busy} onClick={load}>刷新</button>
        </div>
      </div>
      {err && <p className="dh-err">{err}</p>}
      {loading && !agents.length && <p className="empty">加载中…</p>}
      {!loading && agents.length === 0 && <p className="empty">暂无数字人，点击「一键招聘」创建</p>}
      <div className="dh-grid">
        {pageItems.map((a) => (
          <article key={a.id} className={`dh-card ${a.is_receptionist ? 'is-recv' : ''} ${selected.has(a.id) ? 'is-selected' : ''}`}>
            {a.is_receptionist && <span className="dh-badge">前台助理</span>}
            <label className="dh-select">
              <input type="checkbox" checked={selected.has(a.id)} onChange={() => toggleSelect(a.id)} disabled={busy} />
            </label>
            <div className="dh-avatar">
              <img src={agentAvatarUrl(a)} alt="" />
            </div>
            <h3 className="dh-name" title={a.name}>{a.name}</h3>
            <p className="dh-folder">{a.folder_name || (a.folder_id ? `组 #${a.folder_id}` : '未入组')}</p>
            <p className="dh-engine">{a.engine}</p>
            <div className="dh-card-actions">
              <button type="button" className={`dh-recv-btn ${a.is_receptionist ? 'on' : ''}`} disabled={busy} onClick={() => toggleReceptionist(a)}>
                {a.is_receptionist ? '取消前台' : '设为前台助理'}
              </button>
              <button type="button" className="dh-icon-btn" title="设置" disabled={busy} onClick={() => onOpenSettings?.(a)}>
                <GearIcon />
              </button>
              <button type="button" className="dh-icon-btn danger" title="解聘" disabled={busy} onClick={() => deleteOne(a)}>
                <TrashIcon />
              </button>
            </div>
          </article>
        ))}
      </div>
      {agents.length > PAGE_SIZE && (
        <div className="dh-pager">
          <button type="button" disabled={page <= 1 || busy} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</button>
          <span>{Math.min(page, totalPages)} / {totalPages}（共 {agents.length}）</span>
          <button type="button" disabled={page >= totalPages || busy} onClick={() => setPage((p) => Math.min(totalPages, p + 1))}>下一页</button>
        </div>
      )}
    </section>
  )
}
