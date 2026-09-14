import { useEffect, useState } from 'react'

/** 授权剩余秒数 */
export function remainSec(p) {
  if (!p?.deadline) return 999
  return Math.max(0, Math.floor((new Date(p.deadline).getTime() - Date.now()) / 1000))
}

/** 风险中文 */
export function riskLabel(r) {
  return ({ low: '低风险', mid: '中风险', high: '高风险' })[r] || '需注意'
}

/** 命令框：按风险着色 + 底栏含义 + 一键复制 */
export function CmdBlock({ content, risk, meaning, decision }) {
  const r = risk || (decision === 'deny' ? 'high' : 'mid')
  const riskText = ({ low: '无害', mid: '需注意', high: '高风险' })[r] || '需注意'
  async function copy() {
    try {
      await navigator.clipboard.writeText(content || '')
    } catch {
      const ta = document.createElement('textarea')
      ta.value = content || ''
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      ta.remove()
    }
  }
  return (
    <div className={`cmd-block risk-${r} ${decision === 'deny' ? 'denied' : ''}`}>
      <div className="cmd-block-main">
        <code>{content}</code>
        <button type="button" className="cmd-copy" onClick={copy} title="复制命令">复制</button>
      </div>
      {(meaning || risk) && (
        <div className="cmd-meaning-bar">
          <span className="cmd-risk-dot">{riskText}</span>
          <span>{meaning || `风险等级：${riskText}`}</span>
        </div>
      )}
    </div>
  )
}

/**
 * 权限授权卡（Agent 对话 / 团队编排共用）：风险色 + 中文解释 + 同意/拒绝。
 */
export function PermissionCard({ p, more = 0, busy, onDecide, title }) {
  const [, setTick] = useState(0)
  useEffect(() => {
    const t = setInterval(() => setTick((v) => v + 1), 1000)
    return () => clearInterval(t)
  }, [])
  if (!p) return null
  const left = remainSec(p)
  return (
    <div className="perm-card">
      <div className="perm-title">
        {title || '🔐 申请使用工具,需要你授权'}
        <span className={`perm-count ${left <= 10 ? 'urgent' : ''}`}>剩余 {left}s</span>
      </div>
      {p.note && <div className="perm-note">{p.note}</div>}
      <div className="perm-tool">{p.tool_name}</div>
      <CmdBlock content={p.summary} risk={p.risk} meaning={p.meaning} decision="ask" />
      {(p.meaning || p.risk) && (
        <div className={`perm-risk risk-${p.risk || 'mid'}`}>
          ⚠️ 风险：<b>{riskLabel(p.risk)}</b>
          {p.meaning ? ` · ${p.meaning}` : ''}
        </div>
      )}
      {more > 0 && <div className="perm-more">+{more} 条排队中</div>}
      <div className="perm-actions">
        <button type="button" className="deny" disabled={busy} onClick={() => onDecide(p.request_id, 'deny')}>拒绝</button>
        <button type="button" className="allow" disabled={busy} onClick={() => onDecide(p.request_id, 'allow')}>仅本次同意</button>
      </div>
      <div className="perm-hint">授权仅对本次调用生效,超时将自动拒绝。绿/黄/红表示风险：无害 / 需注意 / 高风险。</div>
    </div>
  )
}
