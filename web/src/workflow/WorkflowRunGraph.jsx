import { useMemo } from 'react'
import {
  ReactFlow, Background, Controls, MiniMap, ReactFlowProvider,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { wfNodeTypes, minimapNodeColor } from './WfNodes.jsx'
import { StatusChip } from './NodeRunPanel.jsx'

/** 按最新 attempt 汇总节点状态 */
export function latestStatusByNode(execs = []) {
  const map = {}
  for (const ex of execs) {
    const prev = map[ex.node_id]
    if (!prev || ex.attempt >= prev.attempt) map[ex.node_id] = ex
  }
  return map
}

/**
 * 运行态只读画布：成功绿框 / 失败红框 / 运行中高亮。
 */
function RunGraphInner({ graphJSON, execs, selectedNodeId, onSelectNode }) {
  const byNode = useMemo(() => latestStatusByNode(execs), [execs])
  const { nodes, edges } = useMemo(() => {
    let g = { nodes: [], edges: [] }
    try {
      g = typeof graphJSON === 'string' ? JSON.parse(graphJSON || '{}') : (graphJSON || {})
    } catch { /* empty */ }
    const nodes = (g.nodes || []).map((n) => {
      const ex = byNode[n.id]
      const st = ex?.status || ''
      return {
        ...n,
        type: n.type || 'agent',
        selected: selectedNodeId === n.id,
        data: {
          ...(n.data || {}),
          runStatus: st,
          runAttempt: ex?.attempt,
        },
        className: [
          n.className,
          st === 'success' ? 'wf-run-ok' : '',
          st === 'failed' || st === 'error' || st === 'interrupted' ? 'wf-run-fail' : '',
          st === 'running' || st === 'waiting' ? 'wf-run-live' : '',
          selectedNodeId === n.id ? 'wf-run-selected' : '',
        ].filter(Boolean).join(' '),
      }
    })
    return { nodes, edges: g.edges || [] }
  }, [graphJSON, byNode, selectedNodeId])

  return (
    <div className="wf-run-canvas wf-flow">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={wfNodeTypes}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable
        panOnDrag={[1, 2]}
        onNodeClick={(_, n) => onSelectNode?.(n.id)}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={16} color="#1e293b" />
        <Controls showInteractive={false} />
        <MiniMap
          nodeColor={minimapNodeColor}
          nodeStrokeColor="#0f172a"
          nodeBorderRadius={6}
          maskColor="rgba(7, 11, 20, 0.72)"
          pannable
          zoomable
        />
      </ReactFlow>
    </div>
  )
}

export default function WorkflowRunGraph(props) {
  return (
    <ReactFlowProvider>
      <RunGraphInner {...props} />
    </ReactFlowProvider>
  )
}

/** 执行列表项上的状态芯片（供列表复用） */
export function ExecStatusRow({ ex, active, onClick }) {
  return (
    <button type="button" className={`wf-exec-item ${active ? 'active' : ''} st-border-${ex.status}`} onClick={onClick}>
      <span>{ex.node_type} · {ex.node_id}</span>
      <span className="wf-exec-meta">
        <StatusChip status={ex.status} />
        <span className="muted">#{ex.attempt}</span>
      </span>
    </button>
  )
}
