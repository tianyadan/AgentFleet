import { useEffect, useMemo, useRef, useState } from 'react'
import { PermissionCard } from '../PermissionCard.jsx'

const API = '/api'

function parseEvents(raw) {
  if (!raw) return []
  try {
    const v = typeof raw === 'string' ? JSON.parse(raw) : raw
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

function parseOutput(raw) {
  if (!raw) return null
  try {
    return typeof raw === 'string' ? JSON.parse(raw) : raw
  } catch {
    return { text: String(raw) }
  }
}

/** 节点状态彩色小标签 */
export function StatusChip({ status }) {
  const s = String(status || 'idle')
  const zh = ({
    running: '运行中', pending: '排队中', waiting: '等待中', waiting_recovery: '恢复中',
    success: '成功', failed: '失败', error: '错误',
    stopped: '已停止', interrupted: '已中断', skipped: '已跳过',
  })[s] || s
  return <span className={`wf-status-chip st-${s}`}>{zh}</span>
}

/**
 * 运行态节点侧栏：过程流 / 权限 / 结果 / 错误 / 重试与补充执行。
 */
export default function NodeRunPanel({
  exec, authHeaders, onUnauthorized, canRetry, onRetry, onRerunWithPrompt, busy,
}) {
  const [extra, setExtra] = useState('')
  const [perm, setPerm] = useState(null)
  const [permBusy, setPermBusy] = useState(false)
  const eventBoxRef = useRef(null)

  const events = useMemo(() => parseEvents(exec?.events_json), [exec?.events_json])
  const output = useMemo(() => parseOutput(exec?.output_json), [exec?.output_json])
  const convID = exec?.conversation_id || 0
  const failed = exec?.status === 'failed'
  const running = exec?.status === 'running'

  // 过程区自动滚到底
  useEffect(() => {
    const el = eventBoxRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [events.length, exec?.id, exec?.status])

  useEffect(() => {
    if (!convID) { setPerm(null); return undefined }
    let stop = false
    async function pull() {
      try {
        const r = await fetch(`${API}/permissions/pending?conversation_id=${convID}`, { headers: authHeaders() })
        if (r.status === 401) { onUnauthorized?.(); return }
        const d = await r.json()
        const items = Array.isArray(d.items) ? d.items : []
        if (!stop) setPerm(items[0] || null)
      } catch {
        if (!stop) setPerm(null)
      }
    }
    pull()
    const t = setInterval(pull, 1500)
    return () => { stop = true; clearInterval(t) }
  }, [convID, authHeaders, onUnauthorized])

  async function decide(requestId, behavior) {
    if (!requestId || permBusy) return
    setPermBusy(true)
    try {
      const res = await fetch(`${API}/permissions/decide`, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ request_id: requestId, behavior }),
      })
      if (res.status === 401) onUnauthorized?.()
      setPerm(null)
    } finally {
      setPermBusy(false)
    }
  }

  if (!exec) {
    return (
      <aside className="wf-panel muted">
        <p>选中左侧执行记录或画布节点查看运行过程</p>
      </aside>
    )
  }

  return (
    <aside className="wf-panel wf-run-panel">
      <div className="wf-panel-head">
        <strong>{exec.node_type} · {exec.node_id}</strong>
        <StatusChip status={exec.status} />
        <span className="muted">#{exec.attempt}</span>
      </div>

      <section className="wf-run-sec">
        <h4>运行过程</h4>
        <ul className="wf-event-list" ref={eventBoxRef}>
          {events.length === 0 && <li className="muted">{running ? '等待事件…' : '暂无过程事件'}</li>}
          {events.map((ev, i) => (
            <li key={i} className={`wf-ev wf-ev-${ev.type || 'info'}`}>
              <span className="wf-ev-type">{labelOf(ev.type)}</span>
              <span>{ev.summary || ev.tool || ''}</span>
            </li>
          ))}
        </ul>
      </section>

      {perm && (
        <section className="wf-run-sec wf-perm-sec">
          <PermissionCard
            p={perm}
            busy={permBusy}
            onDecide={decide}
            title="本节点申请执行命令"
          />
        </section>
      )}

      <section className="wf-run-sec">
        <h4>节点运行结果</h4>
        {output?.text || output?.summary || output?.result ? (
          <>
            {output.summary && <p><strong>summary</strong> · {output.summary}</p>}
            <pre className="wf-json">{output.result || output.text || output.summary}</pre>
            {Array.isArray(output.artifacts) && output.artifacts.length > 0 && (
              <ul className="wf-artifact-list">
                {output.artifacts.map((a, i) => (
                  <li key={i}>[{a.type || 'file'}] {a.label ? `${a.label} · ` : ''}{a.ref}</li>
                ))}
              </ul>
            )}
          </>
        ) : (
          <p className="muted">{running ? '执行中，尚无最终输出' : '无输出'}</p>
        )}
        {output?.status && <p className="muted">判定 · {output.status}</p>}
      </section>

      {(failed || exec.error_text) && (
        <section className="wf-run-sec">
          <h4>节点错误信息</h4>
          <pre className="error">{exec.error_text || '未知错误'}</pre>
        </section>
      )}

      {canRetry && (failed || exec.status === 'success' || exec.status === 'stopped') && (
        <section className="wf-run-sec wf-retry-sec">
          <h4>重新执行</h4>
          <p className="muted">补充内容作为该节点新一轮执行指令，不是继续闲聊。</p>
          <textarea
            rows={4}
            value={extra}
            onChange={(e) => setExtra(e.target.value)}
            placeholder="可选：补充/修改本节点要求…"
            disabled={busy}
          />
          <div className="wf-card-actions">
            <button
              type="button"
              className="ghost"
              disabled={busy}
              onClick={() => onRetry?.(exec.node_id)}
              title="原参数不变，再执行一次"
            >
              重试
            </button>
            <button
              type="button"
              className="primary"
              disabled={busy || !extra.trim()}
              onClick={() => onRerunWithPrompt?.(exec.node_id, extra.trim())}
              title="带新的补充 Prompt 创建新的 attempt"
            >
              修改要求并重新执行
            </button>
          </div>
        </section>
      )}

      {convID > 0 && <p className="muted">会话 #{convID}</p>}
    </aside>
  )
}

function labelOf(t) {
  return ({
    activity: '工具',
    command: '命令',
    permission: '授权',
    permission_auto: '自动',
    output: '输出',
    status: '状态',
    system: '系统',
    error: '错误',
  })[t] || (t || '事件')
}
