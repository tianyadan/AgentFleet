import { useEffect, useState } from 'react'
import { emptyGraph, workflowApi } from './workflowApi.js'
import WorkflowCanvas from './WorkflowCanvas.jsx'
import WorkflowRun from './WorkflowRun.jsx'

/** 团队运行状态文案（对齐智能体：空闲/忙碌/等待/错误） */
function runStatusZh(st) {
  const s = String(st || '')
  if (s === 'running' || s === 'pending') return '忙碌中'
  if (s === 'waiting') return '等待中'
  if (s === 'waiting_recovery') return '恢复中'
  if (s === 'failed' || s === 'interrupted' || s === 'error') return '错误'
  if (s === 'stopped') return '已停止'
  if (s === 'success') return '已完成'
  return '空闲中'
}

function runStatusClass(st) {
  const s = String(st || '')
  if (s === 'running' || s === 'pending') return 'running'
  if (s === 'waiting' || s === 'waiting_recovery') return 'waiting'
  if (s === 'failed' || s === 'interrupted' || s === 'error') return 'error'
  if (s === 'success') return 'success'
  if (s === 'stopped') return 'idle'
  return 'idle'
}

function parseCurrentNodes(run) {
  if (!run?.current_node_ids_json) return []
  try {
    const v = JSON.parse(run.current_node_ids_json)
    return Array.isArray(v) ? v.filter(Boolean) : []
  } catch {
    return []
  }
}

/** 可查看运行详情（含终止/失败/完成） */
function canViewRun(run) {
  if (!run?.id) return false
  return ['pending', 'running', 'waiting', 'waiting_recovery', 'failed', 'interrupted', 'stopped', 'success', 'error'].includes(run.status)
}

/**
 * 团队编排：列表 + Canvas + 运行详情。
 */
export default function WorkflowPanel({ authHeaders, onUnauthorized, jumpRunId = 0, onJumpConsumed }) {
  const api = workflowApi(authHeaders)
  const [items, setItems] = useState([])
  const [agents, setAgents] = useState([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [view, setView] = useState('list')
  const [editing, setEditing] = useState(null)
  const [meta, setMeta] = useState({ name: '', description: '' })
  const [runId, setRunId] = useState(0)
  const [startPrompt, setStartPrompt] = useState('')

  async function load() {
    const d = await api.list()
    if (d.error) { setError(d.error); return }
    setItems(d.items || [])
  }

  async function loadAgents() {
    const r = await fetch('/api/admin/agents', { headers: authHeaders() })
    if (r.status === 401) { onUnauthorized?.(); return }
    const d = await r.json()
    setAgents(d.items || [])
  }

  useEffect(() => {
    load()
    loadAgents()
  }, [])

  // 列表页轮询最新运行状态
  useEffect(() => {
    if (view !== 'list') return undefined
    const t = setInterval(load, 3000)
    return () => clearInterval(t)
  }, [view])

  useEffect(() => {
    if (jumpRunId > 0) {
      setRunId(jumpRunId)
      setView('run')
      onJumpConsumed?.()
    }
  }, [jumpRunId])

  async function createNew() {
    setBusy(true)
    setError('')
    try {
      const d = await api.create({
        name: '新编排',
        description: '',
        graph_json: JSON.stringify(emptyGraph()),
      })
      if (d.error) throw new Error(d.error)
      setEditing(d)
      setMeta({ name: d.name, description: d.description || '' })
      setView('edit')
      await load()
    } catch (e) {
      setError(String(e.message || e))
    } finally {
      setBusy(false)
    }
  }

  async function openEdit(id) {
    const d = await api.get(id)
    if (d.error) { setError(d.error); return }
    setEditing(d.definition)
    setMeta({ name: d.definition.name, description: d.definition.description || '' })
    setView('edit')
  }

  async function save({ name, description, graph_json }) {
    if (!editing?.id) return
    setBusy(true)
    try {
      const d = await api.update(editing.id, { name, description, graph_json })
      if (d.error) throw new Error(d.error)
      setEditing(d)
      await load()
    } catch (e) {
      setError(String(e.message || e))
    } finally {
      setBusy(false)
    }
  }

  async function remove(id) {
    if (!confirm('确认删除该编排？')) return
    await api.remove(id)
    await load()
  }

  async function start(id) {
    const prompt = startPrompt.trim() || window.prompt('本次任务提示词', '') || ''
    if (!prompt.trim()) return
    setBusy(true)
    try {
      const run = await api.start(id, prompt)
      if (run.error) throw new Error(run.error)
      setRunId(run.id)
      setView('run')
      setStartPrompt('')
      await load()
    } catch (e) {
      setError(String(e.message || e))
    } finally {
      setBusy(false)
    }
  }

  function openRun(id) {
    setRunId(id)
    setView('run')
  }

  if (view === 'edit' && editing) {
    return (
      <WorkflowCanvas
        initialGraph={editing.graph_json}
        agents={agents}
        name={meta.name}
        description={meta.description}
        onChangeMeta={(p) => setMeta((m) => ({ ...m, ...p }))}
        onSave={save}
        onBack={() => { setView('list'); load() }}
        saving={busy}
      />
    )
  }

  if (view === 'run' && runId) {
    return (
      <WorkflowRun
        runId={runId}
        authHeaders={authHeaders}
        onUnauthorized={onUnauthorized}
        onBack={() => { setView('list'); load() }}
      />
    )
  }

  return (
    <section className="wf-list">
      <header className="wf-list-head">
        <h2>团队编排</h2>
        <button type="button" className="primary" disabled={busy} onClick={createNew}>新增编排</button>
      </header>
      {error && <pre className="error">{error}</pre>}
      <div className="wf-start-row">
        <input
          value={startPrompt}
          onChange={(e) => setStartPrompt(e.target.value)}
          placeholder="启动时的任务提示词（可选，点启动时再填）"
        />
      </div>
      {items.length === 0 && <p className="empty">还没有编排，点击「新增编排」开始</p>}
      <ul className="wf-cards">
        {items.map((it) => {
          const lr = it.latest_run
          const st = lr?.status
          const nodes = parseCurrentNodes(lr)
          const nodeHint = lr?.fail_node_id
            ? `失败节点 · ${lr.fail_node_id}`
            : (nodes.length ? `当前节点 · ${nodes.join(', ')}` : '')
          return (
            <li key={it.id} className="wf-card">
              <div>
                <div className="wf-card-title">
                  <strong>{it.name}</strong>
                  <span className={`ma-status st-${runStatusClass(st)}`}>{runStatusZh(st)}</span>
                </div>
                <p className="muted">{it.description || '无描述'}</p>
                {nodeHint && <p className="wf-run-hint">{nodeHint}</p>}
                <p className="muted">更新于 {new Date(it.updated_at).toLocaleString()}</p>
              </div>
              <div className="wf-card-actions">
                <button type="button" className="ghost" onClick={() => openEdit(it.id)}>编辑</button>
                <button type="button" className="primary" disabled={busy} onClick={() => start(it.id)}>启动</button>
                {canViewRun(lr) && (
                  <button type="button" className="ghost" onClick={() => openRun(lr.id)}>查看任务状态</button>
                )}
                <button type="button" className="ghost danger-text" onClick={() => remove(it.id)}>删除</button>
              </div>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
