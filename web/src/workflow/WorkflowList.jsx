import { useEffect, useState } from 'react'
import { emptyGraph, workflowApi } from './workflowApi.js'
import WorkflowCanvas from './WorkflowCanvas.jsx'
import WorkflowRun from './WorkflowRun.jsx'
import { IconActivity, IconEdit, IconPlay, IconStop, IconTrash } from './nodeIcons.jsx'

/** 团队运行状态文案 */
function runStatusZh(st) {
  const s = String(st || '')
  if (s === 'running' || s === 'pending') return '运行中'
  if (s === 'waiting') return '等待中'
  if (s === 'waiting_recovery') return '恢复中'
  if (s === 'failed' || s === 'interrupted' || s === 'error') return '报错'
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

/** 协作仍在进行：列表只保留终止 + 任务状态 */
function isLiveRun(run) {
  const s = String(run?.status || '')
  return s === 'pending' || s === 'running' || s === 'waiting' || s === 'waiting_recovery'
}

/**
 * 项目协作：列表 + Canvas + 运行详情。
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
  const [conflictModal, setConflictModal] = useState(null) // { defId, prompt, conflicts }

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
        name: '新协作',
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
    if (!confirm('确认删除该项目协作？')) return
    await api.remove(id)
    await load()
  }

  async function stopLive(runId) {
    if (!runId) return
    setBusy(true)
    setError('')
    try {
      const d = await api.stop(runId)
      if (d.error) throw new Error(d.error)
      await load()
    } catch (e) {
      setError(String(e.message || e))
    } finally {
      setBusy(false)
    }
  }

  async function start(id, force = false) {
    const prompt = startPrompt.trim() || window.prompt('本次任务提示词', '') || ''
    if (!prompt.trim()) return
    setBusy(true)
    setError('')
    try {
      const run = await api.start(id, prompt, force)
      if (run._status === 409 && Array.isArray(run.conflicts)) {
        setConflictModal({ defId: id, prompt, conflicts: run.conflicts })
        return
      }
      if (run.error) throw new Error(run.error)
      setConflictModal(null)
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

  async function confirmTakeover() {
    if (!conflictModal) return
    const { defId, prompt } = conflictModal
    setBusy(true)
    setError('')
    try {
      const run = await api.start(defId, prompt, true)
      if (run.error) throw new Error(run.error)
      setConflictModal(null)
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
        <h2>项目协作</h2>
        <button type="button" className="primary" disabled={busy} onClick={createNew}>新增协作</button>
      </header>
      {error && <pre className="error">{error}</pre>}
      <div className="wf-start-row">
        <input
          value={startPrompt}
          onChange={(e) => setStartPrompt(e.target.value)}
          placeholder="启动时的任务提示词（可选，点启动时再填）"
        />
      </div>
      {items.length === 0 && (
        <div className="empty ma-empty ma-empty-hero" style={{ padding: '32px 16px' }}>
          <div className="ma-empty-icon" aria-hidden="true" />
          <p>还没有项目协作</p>
          <p className="ma-empty-sub">点击「新增协作」组建一条协作流程</p>
        </div>
      )}
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
              <div className="wf-card-body">
                <div className="wf-card-title">
                  <strong>{it.name}</strong>
                  <span className={`wf-run-status st-${runStatusClass(st)}`}>
                    <span className={`wf-status-dot st-${runStatusClass(st)}`} aria-hidden="true" />
                    {runStatusZh(st)}
                  </span>
                </div>
                <p className="muted">{it.description || '无描述'}</p>
                {nodeHint && <p className="wf-run-hint">{nodeHint}</p>}
                <p className="muted">更新于 {new Date(it.updated_at).toLocaleString()}</p>
              </div>
              <div className="wf-card-actions">
                {isLiveRun(lr) ? (
                  <>
                    <button type="button" className="stop wf-icon-btn" disabled={busy} onClick={() => stopLive(lr.id)} title="终止">
                      <IconStop />
                      <span>终止</span>
                    </button>
                    <button type="button" className="ghost wf-icon-btn" onClick={() => openRun(lr.id)} title="查看任务状态">
                      <IconActivity />
                      <span>任务状态</span>
                    </button>
                  </>
                ) : (
                  <>
                    <button type="button" className="primary wf-icon-btn" disabled={busy} onClick={() => start(it.id)} title="启动">
                      <IconPlay />
                      <span>启动</span>
                    </button>
                    <button type="button" className="ghost wf-icon-btn" onClick={() => openEdit(it.id)} title="编辑">
                      <IconEdit />
                      <span>编辑</span>
                    </button>
                    {canViewRun(lr) && (
                      <button type="button" className="ghost wf-icon-btn" onClick={() => openRun(lr.id)} title="查看任务状态">
                        <IconActivity />
                        <span>任务状态</span>
                      </button>
                    )}
                    <button type="button" className="ghost danger-text wf-icon-btn" onClick={() => remove(it.id)} title="删除">
                      <IconTrash />
                      <span>删除</span>
                    </button>
                  </>
                )}
              </div>
            </li>
          )
        })}
      </ul>

      {conflictModal && (
        <div className="modal-mask" onClick={() => !busy && setConflictModal(null)}>
          <div className="modal-card ma-settings" onClick={(e) => e.stopPropagation()}>
            <h2>数字员工忙碌</h2>
            <p>
              {(conflictModal.conflicts || []).map((c) => c.name || `#${c.agent_id}`).join('、')}
              {' '}正在执行其它任务（管理台对话 / 整理记忆 / 其它协作）。
            </p>
            <ul className="wf-conflict-list">
              {(conflictModal.conflicts || []).map((c) => (
                <li key={c.agent_id}>
                  <strong>{c.name || `#${c.agent_id}`}</strong>
                  <span className="muted">
                    {' · '}
                    {c.occupancy?.source_type || ''}
                    {c.occupancy?.task_name ? ` · ${c.occupancy.task_name}` : ''}
                  </span>
                </li>
              ))}
            </ul>
            <p className="muted">继续原任务将不启动本次协作；执行团队协作会终止上述员工当前任务。</p>
            <div className="modal-actions">
              <button type="button" className="ghost" disabled={busy} onClick={() => setConflictModal(null)}>
                继续原任务
              </button>
              <button type="button" className="primary" disabled={busy} onClick={confirmTakeover}>
                {busy ? '启动中…' : '执行团队协作'}
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  )
}
