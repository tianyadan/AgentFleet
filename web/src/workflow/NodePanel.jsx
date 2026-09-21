/** 节点配置侧栏 */
export default function NodePanel({ node, agents = [], onChange, onClose, runExec }) {
  if (!node) {
    return (
      <aside className="wf-panel muted">
        <p>选中节点以配置</p>
      </aside>
    )
  }
  const d = node.data || {}
  return (
    <aside className="wf-panel">
      <div className="wf-panel-head">
        <strong>{d.label || node.type}</strong>
        <button type="button" className="ghost" onClick={onClose}>关闭</button>
      </div>
      <p className="muted">类型 · {node.type}</p>
      {node.type === 'agent' && (
        <>
          <label>
            数字员工
            <select
              value={d.agent_id || ''}
              onChange={(e) => onChange({ agent_id: Number(e.target.value) || 0 })}
            >
              <option value="">选择…</option>
              {agents.map((a) => (
                <option key={a.id} value={a.id}>{a.name} ({a.engine})</option>
              ))}
            </select>
          </label>
          <label>
            职责描述
            <textarea
              rows={6}
              value={d.role_hint || ''}
              onChange={(e) => onChange({ role_hint: e.target.value })}
              placeholder="该数字员工在本节点的职责与提示词…"
            />
          </label>
          <div className="ma-capsule-row">
            <div className="ma-capsule-text">
              <span className="ma-capsule-label">命令自动审核</span>
              <p className="ma-capsule-hint">开启后由 AI 自动放行常规命令；sudo / rm 等敏感命令仍需人工授权</p>
            </div>
            <label className="auto-switch ma-capsule">
              <input
                type="checkbox"
                checked={!!d.auto_review}
                onChange={(e) => onChange({ auto_review: e.target.checked })}
              />
              <span className="slider" />
            </label>
          </div>
        </>
      )}
      {node.type === 'condition' && (
        <>
          <label>
            表达式
            <input
              value={d.expr || ''}
              onChange={(e) => onChange({ expr: e.target.value })}
              placeholder='status == "PASS"'
            />
          </label>
          <label className="ma-capsule">
            <span>AI 判定（预留）</span>
            <input type="checkbox" checked={!!d.ai_enabled} disabled readOnly />
          </label>
        </>
      )}
      {node.type === 'human_review' && (
        <label>
          驳回回流节点 ID
          <input
            value={d.reject_target_node_id || ''}
            onChange={(e) => onChange({ reject_target_node_id: e.target.value })}
            placeholder="例如 ui_design 节点 id"
          />
        </label>
      )}
      {node.type === 'merge' && (
        <>
          <div className="wf-node-help">
            <p className="wf-help-title">使用说明</p>
            <ul className="wf-help-list">
              <li>放在并行分支之后，等待多条入边都成功后再往下走。</li>
              <li>左侧有多个连接点，可分别接各并行分支的输出。</li>
              <li>「等待节点 IDs」可空：表示等待全部入边节点成功；填写则只等待指定节点。</li>
              <li>典型用法：Parallel 拆出 A/B → 各自 Agent → Merge 汇合 → 后续节点。</li>
            </ul>
          </div>
          <label>
            等待节点 IDs（逗号分隔，可空=全部入边）
            <input
              value={(d.wait_for_node_ids || []).join?.(',') || d.wait_for_csv || ''}
              onChange={(e) => onChange({
                wait_for_csv: e.target.value,
                wait_for_node_ids: e.target.value.split(/[,，\s]+/).filter(Boolean),
              })}
            />
          </label>
        </>
      )}
      {runExec && (
        <div className="wf-exec-box">
          <p>状态：{runExec.status}</p>
          {runExec.error_text && <pre className="error">{runExec.error_text}</pre>}
          {runExec.output_json && <pre className="wf-json">{runExec.output_json}</pre>}
          {runExec.conversation_id > 0 && (
            <p className="muted">会话 #{runExec.conversation_id}</p>
          )}
        </div>
      )}
    </aside>
  )
}
