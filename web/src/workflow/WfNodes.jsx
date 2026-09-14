import { memo } from 'react'
import { Handle, Position } from '@xyflow/react'

const ENGINE_CLASS = {
  claude: 'eng-claude',
  codex: 'eng-codex',
  agent: 'eng-cursor',
}

/** 通用工作流节点外观（流程图形状 + Agent 引擎色 + 运行态边框） */
function WfNodeInner({ data, type }) {
  const label = data?.label || type
  const sub = data?.agent_name || data?.expr || data?.role_hint || ''
  const eng = String(data?.engine || '').toLowerCase()
  const engCls = type === 'agent' ? (ENGINE_CLASS[eng] || 'eng-unknown') : ''
  const runSt = data?.runStatus
  return (
    <div className={`wf-node wf-node-${type} ${engCls} ${runSt ? `run-${runSt}` : ''}`.trim()}>
      <Handle type="target" position={Position.Left} />
      <strong>{label}</strong>
      {sub ? <span className="wf-node-sub">{String(sub).slice(0, 48)}</span> : null}
      {type === 'agent' && eng ? <span className="wf-node-eng">{eng}</span> : null}
      {runSt ? <span className={`wf-status-chip st-${runSt}`}>{statusZh(runSt)}</span> : null}
      {type === 'condition' && (
        <>
          <Handle type="source" position={Position.Right} id="true" style={{ top: '35%' }} />
          <Handle type="source" position={Position.Right} id="false" style={{ top: '70%' }} />
        </>
      )}
      {type === 'human_review' && (
        <>
          <Handle type="source" position={Position.Right} id="approve" style={{ top: '35%' }} />
          <Handle type="source" position={Position.Right} id="reject" style={{ top: '70%' }} />
        </>
      )}
      {type !== 'condition' && type !== 'human_review' && type !== 'end' && (
        <Handle type="source" position={Position.Right} />
      )}
    </div>
  )
}

function statusZh(s) {
  return ({
    running: '运行中', pending: '排队', waiting: '等待',
    success: '成功', failed: '失败', stopped: '停止', interrupted: '中断',
  })[s] || s
}

export const wfNodeTypes = {
  start: memo((p) => <WfNodeInner {...p} type="start" />),
  agent: memo((p) => <WfNodeInner {...p} type="agent" />),
  human_review: memo((p) => <WfNodeInner {...p} type="human_review" />),
  condition: memo((p) => <WfNodeInner {...p} type="condition" />),
  parallel: memo((p) => <WfNodeInner {...p} type="parallel" />),
  merge: memo((p) => <WfNodeInner {...p} type="merge" />),
  end: memo((p) => <WfNodeInner {...p} type="end" />),
}

/** MiniMap / 图例着色 */
export function minimapNodeColor(node) {
  const t = node.type || 'agent'
  if (t === 'start') return '#22c55e'
  if (t === 'end') return '#94a3b8'
  if (t === 'condition') return '#fbbf24'
  if (t === 'human_review') return '#f472b6'
  if (t === 'parallel' || t === 'merge') return '#a78bfa'
  if (t === 'agent') {
    const eng = String(node.data?.engine || '').toLowerCase()
    if (eng === 'claude') return '#D97757'
    if (eng === 'codex') return '#10a37f'
    if (eng === 'agent') return '#6366f1'
    return '#38bdf8'
  }
  return '#64748b'
}
