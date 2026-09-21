import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ReactFlow, Background, Controls, MiniMap, addEdge,
  useNodesState, useEdgesState, SelectionMode,
  ReactFlowProvider,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { emptyGraph, NODE_TYPES } from './workflowApi.js'
import { wfNodeTypes, minimapNodeColor } from './WfNodes.jsx'
import NodePanel from './NodePanel.jsx'
import { NODE_TYPE_ICONS, IconSettings } from './nodeIcons.jsx'

let nid = 1
function newId(prefix) {
  nid += 1
  return `${prefix}_${nid}_${Date.now().toString(36)}`
}

const UNDO_MAX = 5

function parseGraph(initialGraph) {
  try {
    const g = typeof initialGraph === 'string' ? JSON.parse(initialGraph || '{}') : (initialGraph || emptyGraph())
    return {
      nodes: (g.nodes || []).map((n) => ({ ...n, type: n.type || 'agent' })),
      edges: g.edges || [],
      settings: g.settings && typeof g.settings === 'object' ? { ...g.settings } : {},
    }
  } catch {
    return emptyGraph()
  }
}

/** 胶囊开关（协作设置） */
function WfCapsuleToggle({ label, checked, onChange, hint }) {
  return (
    <div className="ma-capsule-row">
      <div className="ma-capsule-text">
        <span className="ma-capsule-label">{label}</span>
        {hint ? <p className="ma-capsule-hint">{hint}</p> : null}
      </div>
      <label className="auto-switch ma-capsule">
        <input type="checkbox" checked={!!checked} onChange={(e) => onChange(e.target.checked)} />
        <span className="slider" />
      </label>
    </div>
  )
}

function WorkflowCanvasInner({
  initialGraph, agents = [], name, description,
  onChangeMeta, onSave, onBack, saving,
}) {
  const graph0 = useMemo(() => parseGraph(initialGraph), [initialGraph])

  const [nodes, setNodes, onNodesChange] = useNodesState(graph0.nodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(graph0.edges)
  const [settings, setSettings] = useState(graph0.settings || {})
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [selected, setSelected] = useState(null)
  const undoRef = useRef([])
  const nodesRef = useRef(nodes)
  const edgesRef = useRef(edges)
  useEffect(() => {
    nodesRef.current = nodes
    edgesRef.current = edges
  }, [nodes, edges])

  useEffect(() => {
    setNodes(graph0.nodes)
    setEdges(graph0.edges)
    setSettings(graph0.settings || {})
    undoRef.current = []
  }, [graph0, setNodes, setEdges])

  // 打开编辑时按 agent_id 补齐 avatar_url / 名称 / 引擎
  useEffect(() => {
    if (!agents?.length) return
    setNodes((ns) => ns.map((n) => {
      if (n.type !== 'agent') return n
      const aid = n.data?.agent_id
      if (!aid) return n
      const a = agents.find((x) => String(x.id) === String(aid))
      if (!a) return n
      return {
        ...n,
        data: {
          ...n.data,
          agent_name: a.name || n.data.agent_name,
          engine: a.engine || n.data.engine,
          avatar_url: a.avatar_url || n.data.avatar_url || '',
          label: a.name || n.data.label,
        },
      }
    }))
  }, [agents, setNodes])

  const pushUndo = useCallback(() => {
    const snap = {
      nodes: structuredClone(nodesRef.current),
      edges: structuredClone(edgesRef.current),
    }
    undoRef.current = [...undoRef.current, snap].slice(-UNDO_MAX)
  }, [])

  const undo = useCallback(() => {
    const stack = undoRef.current
    if (!stack.length) return
    const prev = stack[stack.length - 1]
    undoRef.current = stack.slice(0, -1)
    setNodes(prev.nodes)
    setEdges(prev.edges)
  }, [setNodes, setEdges])

  const wrapNodesChange = useCallback((changes) => {
    if (changes.some((c) => c.type === 'remove' || (c.type === 'position' && c.dragging === false))) {
      pushUndo()
    }
    onNodesChange(changes)
  }, [onNodesChange, pushUndo])

  const wrapEdgesChange = useCallback((changes) => {
    if (changes.some((c) => c.type === 'remove')) pushUndo()
    onEdgesChange(changes)
  }, [onEdgesChange, pushUndo])

  const onConnect = useCallback((conn) => {
    pushUndo()
    setEdges((eds) => addEdge({ ...conn, id: newId('e') }, eds))
  }, [setEdges, pushUndo])

  function addNode(type) {
    pushUndo()
    const id = newId(type)
    setNodes((ns) => [...ns, {
      id,
      type,
      position: { x: 180 + Math.random() * 200, y: 80 + Math.random() * 220 },
      data: { label: NODE_TYPES.find((t) => t.type === type)?.label || type },
    }])
  }

  function updateSelectedData(patch) {
    if (!selected) return
    pushUndo()
    setNodes((ns) => ns.map((n) => {
      if (n.id !== selected.id) return n
      const data = { ...n.data, ...patch }
      if (patch.agent_id) {
        const a = agents.find((x) => String(x.id) === String(patch.agent_id))
        if (a) {
          data.agent_name = a.name
          data.label = a.name
          data.engine = a.engine
          data.avatar_url = a.avatar_url || ''
        }
      }
      return { ...n, data }
    }))
    setSelected((s) => (s ? { ...s, data: { ...s.data, ...patch } } : s))
  }

  function save() {
    onSave?.({
      name,
      description,
      graph_json: JSON.stringify({ nodes, edges, settings: settings || {} }),
    })
  }

  useEffect(() => {
    function onKey(e) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'z' && !e.shiftKey) {
        const tag = (e.target?.tagName || '').toLowerCase()
        if (tag === 'input' || tag === 'textarea') return
        e.preventDefault()
        undo()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [undo])

  const selNode = selected ? nodes.find((n) => n.id === selected.id) : null
  const allowAll = !!settings.allow_all_commands

  return (
    <div className="wf-canvas-wrap">
      <header className="wf-canvas-head">
        <button type="button" className="ghost" onClick={onBack}>← 返回</button>
        <input
          className="wf-name-input"
          value={name}
          onChange={(e) => onChangeMeta?.({ name: e.target.value })}
          placeholder="协作名称"
        />
        <input
          className="wf-desc-input"
          value={description}
          onChange={(e) => onChangeMeta?.({ description: e.target.value })}
          placeholder="描述"
        />
        <button type="button" className="ghost" onClick={undo} title="Ctrl+Z">撤销</button>
        <button type="button" className="ghost wf-icon-btn" onClick={() => setSettingsOpen(true)} title="协作设置">
          <IconSettings />
          <span>设置</span>
        </button>
        <button type="button" className="primary" disabled={saving} onClick={save}>
          {saving ? '保存中…' : '保存'}
        </button>
      </header>
      <div className="wf-palette">
        {NODE_TYPES.map((t) => {
          const Icon = NODE_TYPE_ICONS[t.type]
          return (
            <button key={t.type} type="button" className="ghost wf-icon-btn" onClick={() => addNode(t.type)}>
              {Icon ? <Icon /> : null}
              <span>{t.label}</span>
            </button>
          )
        })}
      </div>
      <div className="wf-canvas-body">
        <div className="wf-flow-col">
          <div className="wf-flow">
            <ReactFlow
              nodes={nodes}
              edges={edges}
              onNodesChange={wrapNodesChange}
              onEdgesChange={wrapEdgesChange}
              onConnect={onConnect}
              nodeTypes={wfNodeTypes}
              onNodeClick={(_, n) => setSelected(n)}
              onPaneClick={() => setSelected(null)}
              fitView
              selectionOnDrag
              selectionMode={SelectionMode.Partial}
              panOnDrag={[1, 2]}
              deleteKeyCode={['Backspace', 'Delete']}
              multiSelectionKeyCode="Shift"
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
          <p className="muted wf-hint-below">
            拖拽框选 · Delete 删除 · Ctrl+Z 撤销 · 中/右键平移 · 拖动右下角小图例移动视角
          </p>
        </div>
        <NodePanel
          node={selNode}
          agents={agents}
          onChange={updateSelectedData}
          onClose={() => setSelected(null)}
        />
      </div>

      {settingsOpen && (
        <div className="modal-mask" onClick={() => setSettingsOpen(false)}>
          <div className="modal-card ma-settings" onClick={(e) => e.stopPropagation()}>
            <h2>协作设置</h2>
            <p className="muted">以下设置对整个项目协作生效。</p>
            <WfCapsuleToggle
              label="允许执行所有命令"
              checked={allowAll}
              onChange={(c) => setSettings((s) => ({ ...s, allow_all_commands: c }))}
              hint="开启后，协作内所有命令自动放行且无需弹窗；rm 仍需人工确认。"
            />
            <div className="modal-actions">
              <button type="button" onClick={() => setSettingsOpen(false)}>关闭</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default function WorkflowCanvas(props) {
  return (
    <ReactFlowProvider>
      <WorkflowCanvasInner {...props} />
    </ReactFlowProvider>
  )
}
