/** 按最新 attempt 汇总节点执行（与 WorkflowRunGraph 一致） */
function latestStatusByNode(execs = []) {
  const map = {}
  for (const ex of execs) {
    const prev = map[ex.node_id]
    if (!prev || ex.attempt >= prev.attempt) map[ex.node_id] = ex
  }
  return map
}

/**
 * 计算运行详情下次应选中的执行。
 * followLive=false 表示用户已手动点选，轮询不得抢回 running 节点。
 */
export function nextRunSelection(execs, { followLive, curId, nodeId }) {
  const list = execs || []
  if (!followLive) {
    if (curId && list.some((e) => e.id === curId)) {
      const keep = list.find((e) => e.id === curId)
      return { id: keep.id, nodeId: keep.node_id }
    }
    const latest = nodeId ? latestStatusByNode(list)[nodeId] : null
    if (latest) return { id: latest.id, nodeId: latest.node_id }
    return { id: curId || 0, nodeId: nodeId || '' }
  }
  const running = [...list].reverse().find((e) => e.status === 'running')
  if (running) return { id: running.id, nodeId: running.node_id }
  if (curId && list.some((e) => e.id === curId)) {
    const keep = list.find((e) => e.id === curId)
    return { id: keep.id, nodeId: keep.node_id }
  }
  const failed = [...list].reverse().find((e) => e.status === 'failed')
  const pick = failed || list[list.length - 1]
  return { id: pick?.id || 0, nodeId: pick?.node_id || '' }
}
