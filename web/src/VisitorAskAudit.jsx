import { useEffect, useState } from 'react'

const API = '/api'

function fmtTokens(n) {
  n = Number(n) || 0
  if (n < 1000) return String(n)
  if (n < 10000) return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k`
  return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k`
}

function fmtDuration(ms) {
  ms = Number(ms) || 0
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function statusLabel(s) {
  const m = {
    success: 'SUCCESS',
    error: 'ERROR',
    cancelled: 'CANCELLED',
    processing: 'PROCESSING',
  }
  return m[s] || (s || '—').toUpperCase()
}

/**
 * 后台「对话审计」：前台 Ask 一轮一条。
 */
export default function VisitorAskAudit({ authHeaders, onUnauthorized, active }) {
  const [items, setItems] = useState([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(10)
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')
  const [keyword, setKeyword] = useState('')
  const [visitorName, setVisitorName] = useState('')
  const [status, setStatus] = useState('')
  const [detail, setDetail] = useState(null)
  const [detailLoading, setDetailLoading] = useState(false)

  async function load(p = page) {
    setLoading(true)
    setErr('')
    try {
      const q = new URLSearchParams({
        page: String(p),
        pageSize: String(pageSize),
      })
      if (keyword.trim()) q.set('keyword', keyword.trim())
      if (visitorName.trim()) q.set('visitorName', visitorName.trim())
      if (status) q.set('status', status)
      const res = await fetch(`${API}/admin/visitor-ask-logs?${q}`, { headers: authHeaders() })
      if (res.status === 401) { onUnauthorized?.(); return }
      const d = await res.json()
      if (!res.ok) throw new Error(d.error || '加载失败')
      setItems(d.items || [])
      setTotal(d.total || 0)
      setPage(d.page || p)
    } catch (e) {
      setErr(String(e.message || e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (active) load(1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active])

  async function openDetail(id) {
    setDetailLoading(true)
    setDetail(null)
    try {
      const res = await fetch(`${API}/admin/visitor-ask-logs/${id}`, { headers: authHeaders() })
      if (res.status === 401) { onUnauthorized?.(); return }
      const d = await res.json()
      if (!res.ok) throw new Error(d.error || '加载详情失败')
      setDetail(d)
    } catch (e) {
      setErr(String(e.message || e))
    } finally {
      setDetailLoading(false)
    }
  }

  const pages = Math.max(1, Math.ceil(total / pageSize) || 1)

  return (
    <section className="list audit-panel">
      <div className="audit-toolbar">
        <h2>对话审计</h2>
        <p className="empty-hint">前台助理每一轮 Ask 一条记录 · 每页 {pageSize} 条 · 共 {total} 条</p>
      </div>

      <div className="audit-filters">
        <input
          value={visitorName}
          onChange={(e) => setVisitorName(e.target.value)}
          placeholder="访客姓名"
        />
        <input
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder="问题关键词"
        />
        <select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">全部状态</option>
          <option value="success">SUCCESS</option>
          <option value="error">ERROR</option>
          <option value="cancelled">CANCELLED</option>
          <option value="processing">PROCESSING</option>
        </select>
        <button type="button" className="new" disabled={loading} onClick={() => load(1)}>查询</button>
        <button type="button" className="ghost" disabled={loading} onClick={() => load(page)}>刷新</button>
      </div>

      {err && <p className="error">{err}</p>}

      <div className="audit-table-wrap">
        <table className="audit-table">
          <thead>
            <tr>
              <th>访客</th>
              <th>问题</th>
              <th>Agent</th>
              <th>Token</th>
              <th>耗时</th>
              <th>状态</th>
              <th>时间</th>
            </tr>
          </thead>
          <tbody>
            {!loading && items.length === 0 && (
              <tr><td colSpan={7} className="empty">暂无审计记录</td></tr>
            )}
            {items.map((it) => (
              <tr key={it.id} className="audit-row" onClick={() => openDetail(it.id)}>
                <td>
                  <div className="audit-visitor">{it.visitorName || '—'}</div>
                  <div className="audit-ip">{it.ipMasked || ''}</div>
                </td>
                <td className="audit-q" title={it.question}>{it.question || '—'}</td>
                <td>{it.agentName || '—'}</td>
                <td>{fmtTokens(it.totalTokens)}</td>
                <td>{fmtDuration(it.durationMs)}</td>
                <td><span className={`audit-status st-${it.status}`}>{statusLabel(it.status)}</span></td>
                <td>{it.createdAt ? new Date(it.createdAt).toLocaleString() : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="pager">
        <button type="button" disabled={page <= 1 || loading} onClick={() => load(page - 1)}>上一页</button>
        <span>第 {page} / {pages} 页</span>
        <button type="button" disabled={page >= pages || loading} onClick={() => load(page + 1)}>下一页</button>
      </div>

      {(detail || detailLoading) && (
        <div className="audit-drawer-mask" onClick={() => !detailLoading && setDetail(null)}>
          <aside className="audit-drawer" onClick={(e) => e.stopPropagation()}>
            <div className="audit-drawer-head">
              <h3>审计详情</h3>
              <button type="button" className="ghost" onClick={() => setDetail(null)}>关闭</button>
            </div>
            {detailLoading && <p className="empty">加载中…</p>}
            {detail && (
              <div className="audit-detail">
                <section>
                  <h4>访客信息</h4>
                  <p><b>名称：</b>{detail.visitorName || '—'}</p>
                  <p><b>Visitor ID：</b>{detail.visitorPublicId || detail.visitorId}</p>
                  <p><b>IP：</b>{detail.ip || '—'}</p>
                  <p><b>User-Agent：</b>{detail.userAgent || '—'}</p>
                  <p><b>时间：</b>{detail.createdAt ? new Date(detail.createdAt).toLocaleString() : '—'}</p>
                </section>
                <section>
                  <h4>用户问题</h4>
                  <pre className="audit-pre">{detail.question || '—'}</pre>
                </section>
                <section>
                  <h4>AI 回答</h4>
                  <pre className="audit-pre">{detail.answer || '—'}</pre>
                </section>
                <section>
                  <h4>调用信息</h4>
                  <p><b>Agent：</b>{detail.agentName || '—'}</p>
                  <p><b>Engine：</b>{detail.engine || '—'}</p>
                  <p><b>Conversation：</b>#{detail.conversationId}</p>
                  <p><b>User Msg：</b>{detail.userMessageId || 0} · <b>Assistant Msg：</b>{detail.assistantMessageId || 0}</p>
                </section>
                <section>
                  <h4>Token / 性能</h4>
                  <p><b>Input：</b>{detail.inputTokens ?? 0}</p>
                  <p><b>Output：</b>{detail.outputTokens ?? 0}</p>
                  <p><b>Cached：</b>{detail.cachedTokens ?? 0}</p>
                  <p><b>Total：</b>{detail.totalTokens ?? 0}</p>
                  <p><b>耗时：</b>{fmtDuration(detail.durationMs)}</p>
                  <p><b>状态：</b><span className={`audit-status st-${detail.status}`}>{statusLabel(detail.status)}</span></p>
                  {detail.finishedAt && <p><b>结束：</b>{new Date(detail.finishedAt).toLocaleString()}</p>}
                </section>
                {detail.status === 'error' && (
                  <section>
                    <h4>异常信息</h4>
                    <pre className="audit-pre err">{detail.errorMessage || '—'}</pre>
                  </section>
                )}
              </div>
            )}
          </aside>
        </div>
      )}
    </section>
  )
}
