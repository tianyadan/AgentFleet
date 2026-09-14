import { useEffect, useState } from 'react'
import { workflowApi } from './workflowApi.js'
import NodeRunPanel from './NodeRunPanel.jsx'
import WorkflowRunGraph, { ExecStatusRow, latestStatusByNode } from './WorkflowRunGraph.jsx'

function runStatusLabel(st) {
  return ({
    pending: '排队中', running: '运行中', waiting: '等待审核',
    waiting_recovery: '恢复中', success: '已完成', failed: '失败',
    stopped: '已停止', interrupted: '已中断',
  })[st] || st
}

/** 运行详情：画布状态、审核、节点过程、按节点重试 */
export default function WorkflowRun({ runId, authHeaders, onUnauthorized, onBack }) {
  const api = workflowApi(authHeaders)
  const [detail, setDetail] = useState(null)
  const [rejectPrompt, setRejectPrompt] = useState('')
  const [selectedId, setSelectedId] = useState(0)
  const [selectedNodeId, setSelectedNodeId] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function refresh() {
    const d = await api.getRun(runId)
    if (d.error) { setError(d.error); return }
    setDetail(d)
    const execs = d.executions || []
    setSelectedId((cur) => {
      const running = [...execs].reverse().find((e) => e.status === 'running')
      if (running) {
        setSelectedNodeId(running.node_id)
        return running.id
      }
      if (cur && execs.some((e) => e.id === cur)) return cur
      const failed = [...execs].reverse().find((e) => e.status === 'failed')
      const pick = failed || execs[execs.length - 1]
      if (pick) setSelectedNodeId(pick.node_id)
      return pick?.id || 0
    })
  }

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 1500)
    return () => clearInterval(t)
  }, [runId])

  if (!detail?.run) {
    return (
      <section className="wf-run">
        <button type="button" className="ghost" onClick={onBack}>← 返回</button>
        {error ? <pre className="error">{error}</pre> : <p className="thinking">加载运行…</p>}
      </section>
    )
  }

  const run = detail.run
  const execs = detail.executions || []
  const waiting = run.status === 'waiting'
  const selectedExec = execs.find((e) => e.id === selectedId) || null
  const canRetry = !['running', 'waiting', 'pending'].includes(run.status)
  const byNode = latestStatusByNode(execs)

  function selectNode(nodeId) {
    setSelectedNodeId(nodeId)
    const ex = byNode[nodeId]
    if (ex) setSelectedId(ex.id)
  }

  async function retryNode(nodeId, extra = '') {
    setBusy(true)
    setError('')
    try {
      const d = await api.retryNode(run.id, nodeId, extra)
      if (d.error) throw new Error(d.error)
      await refresh()
    } catch (e) {
      setError(String(e.message || e))
    } finally {
      setBusy(false)
    }
  }

  async function resumeWorkflow() {
    setBusy(true)
    setError('')
    try {
      const d = await api.resume(run.id)
      if (d.error) throw new Error(d.error)
      await refresh()
    } catch (e) {
      setError(String(e.message || e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="wf-run">
      <header className="wf-run-head">
        <button type="button" className="ghost" onClick={onBack}>← 返回</button>
        <h2>{detail.definition?.name || '运行'} · #{run.id}</h2>
        <span className={`wf-status-chip st-${run.status}`}>{runStatusLabel(run.status)}</span>
      </header>
      <p className="wf-prompt"><strong>任务：</strong>{run.input_prompt}</p>
      {run.fail_reason && <pre className="error">失败节点 {run.fail_node_id}: {run.fail_reason}</pre>}
      {error && <pre className="error">{error}</pre>}

      <div className="wf-run-actions">
        {(run.status === 'running' || run.status === 'waiting' || run.status === 'waiting_recovery') && (
          <button type="button" className="stop" onClick={() => api.stop(run.id).then(refresh)}>终止</button>
        )}
        {run.status === 'failed' && run.fail_node_id && (
          <button type="button" className="ghost" disabled={busy} onClick={() => retryNode(run.fail_node_id)}>
            从失败节点重试
          </button>
        )}
        {(run.status === 'interrupted' || run.status === 'waiting_recovery') && (
          <button type="button" className="primary" disabled={busy} onClick={resumeWorkflow}>
            恢复工作流
          </button>
        )}
      </div>

      {waiting && (
        <div className="wf-review-box">
          <h3>人工审核</h3>
          <textarea rows={3} value={rejectPrompt} onChange={(e) => setRejectPrompt(e.target.value)} placeholder="驳回时填写意见" />
          <div className="wf-card-actions">
            <button type="button" className="primary" onClick={() => api.review(run.id, 'approve').then(refresh)}>通过</button>
            <button type="button" className="ghost" onClick={() => api.review(run.id, 'reject', rejectPrompt).then(refresh)}>驳回</button>
            <button type="button" className="stop" onClick={() => api.review(run.id, 'abort').then(refresh)}>终止工作流</button>
          </div>
        </div>
      )}

      <WorkflowRunGraph
        graphJSON={detail.definition?.graph_json}
        execs={execs}
        selectedNodeId={selectedNodeId}
        onSelectNode={selectNode}
      />

      <div className="wf-run-grid">
        <ul className="wf-exec-list">
          {execs.map((ex) => (
            <li key={ex.id}>
              <ExecStatusRow
                ex={ex}
                active={selectedId === ex.id}
                onClick={() => { setSelectedId(ex.id); setSelectedNodeId(ex.node_id) }}
              />
            </li>
          ))}
        </ul>
        <NodeRunPanel
          exec={selectedExec}
          authHeaders={authHeaders}
          onUnauthorized={onUnauthorized}
          canRetry={canRetry}
          busy={busy}
          onRetry={(nodeId) => retryNode(nodeId, '')}
          onRerunWithPrompt={(nodeId, prompt) => retryNode(nodeId, prompt)}
        />
      </div>
    </section>
  )
}
