import { useEffect, useState } from 'react'

const API = '/api'

/** 设置图标 */
function GearIcon() {
  return (
    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 1v2M12 21v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M1 12h2M21 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4" />
    </svg>
  )
}

/** 删除图标 */
function TrashIcon() {
  return (
    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M3 6h18" />
      <path d="M8 6V4h8v2" />
      <path d="M19 6l-1 14H6L5 6" />
      <path d="M10 11v6M14 11v6" />
    </svg>
  )
}

/**
 * 数字人管理：卡片网格、前台助理、多选解聘。
 * onOpenSettings(agent) / onHire 由父级打开现有招聘/设置流程。
 */
export default function DigitalHumanCards({ authHeaders, onUnauthorized, onOpenSettings, onHire, active }) {
  const [agents, setAgents] = useState([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState(() => new Set())
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  async function load() {
    setLoading(true)
    setErr('')
    try {
      const res = await fetch(`${API}/admin/agents`, { headers: authHeaders() })
      if (res.status === 401) { onUnauthorized?.(); return }
      const d = await res.json()
      setAgents(Array.isArray(d) ? d : (d.items || []))
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

  function toggleAll() {
    if (selected.size === agents.length) setSelected(new Set())
    else setSelected(new Set(agents.map((a) => a.id)))
  }

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
            <input type="checkbox" checked={agents.length > 0 && selected.size === agents.length} onChange={toggleAll} disabled={!agents.length || busy} />
            全选
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
        {agents.map((a) => (
          <article key={a.id} className={`dh-card ${a.is_receptionist ? 'is-recv' : ''} ${selected.has(a.id) ? 'is-selected' : ''}`}>
            {a.is_receptionist && <span className="dh-badge">前台助理</span>}
            <label className="dh-select">
              <input type="checkbox" checked={selected.has(a.id)} onChange={() => toggleSelect(a.id)} disabled={busy} />
            </label>
            <div className="dh-avatar">
              {a.avatar_url ? <img src={a.avatar_url} alt="" /> : (a.name || '?').slice(0, 1)}
            </div>
            <h3 className="dh-name">{a.name}</h3>
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
    </section>
  )
}
