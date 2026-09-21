const API = '/api'

/** Workflow REST 封装 */
export function workflowApi(authHeaders) {
  const hdr = (extra = {}) => authHeaders(extra)
  return {
    async list() {
      const r = await fetch(`${API}/admin/workflows`, { headers: hdr() })
      return r.json()
    },
    async get(id) {
      const r = await fetch(`${API}/admin/workflows/${id}`, { headers: hdr() })
      return r.json()
    },
    async create(body) {
      const r = await fetch(`${API}/admin/workflows`, {
        method: 'POST', headers: hdr({ 'Content-Type': 'application/json' }),
        body: JSON.stringify(body),
      })
      return r.json()
    },
    async update(id, body) {
      const r = await fetch(`${API}/admin/workflows/${id}`, {
        method: 'PATCH', headers: hdr({ 'Content-Type': 'application/json' }),
        body: JSON.stringify(body),
      })
      return r.json()
    },
    async remove(id) {
      const r = await fetch(`${API}/admin/workflows/${id}`, { method: 'DELETE', headers: hdr() })
      return r.json()
    },
    async start(id, input_prompt, force_takeover = false) {
      const r = await fetch(`${API}/admin/workflows/${id}/runs`, {
        method: 'POST', headers: hdr({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ input_prompt, force_takeover: !!force_takeover }),
      })
      const d = await r.json()
      return { ...d, _status: r.status }
    },
    async getRun(runId) {
      const r = await fetch(`${API}/admin/workflows/runs/${runId}`, { headers: hdr() })
      return r.json()
    },
    async stop(runId) {
      const r = await fetch(`${API}/admin/workflows/runs/${runId}/stop`, { method: 'POST', headers: hdr() })
      return r.json()
    },
    async retry(runId) {
      const r = await fetch(`${API}/admin/workflows/runs/${runId}/retry`, { method: 'POST', headers: hdr() })
      return r.json()
    },
    async resume(runId) {
      const r = await fetch(`${API}/admin/workflows/runs/${runId}/resume`, { method: 'POST', headers: hdr() })
      return r.json()
    },
    async retryNode(runId, nodeId, extra_prompt = '') {
      const r = await fetch(`${API}/admin/workflows/runs/${runId}/nodes/${encodeURIComponent(nodeId)}/retry`, {
        method: 'POST', headers: hdr({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ extra_prompt }),
      })
      return r.json()
    },
    async review(runId, action, reject_prompt = '') {
      const r = await fetch(`${API}/admin/workflows/runs/${runId}/review`, {
        method: 'POST', headers: hdr({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ action, reject_prompt }),
      })
      return r.json()
    },
  }
}

export const NODE_TYPES = [
  { type: 'start', label: 'Start' },
  { type: 'agent', label: 'Agent' },
  { type: 'human_review', label: 'Human Review' },
  { type: 'condition', label: 'Condition' },
  { type: 'parallel', label: 'Parallel' },
  { type: 'merge', label: 'Merge' },
  { type: 'end', label: 'End' },
]

export function emptyGraph() {
  return {
    nodes: [
      { id: 'start', type: 'start', position: { x: 80, y: 160 }, data: { label: 'Start' } },
      { id: 'end', type: 'end', position: { x: 520, y: 160 }, data: { label: 'End' } },
    ],
    edges: [],
    settings: {},
  }
}
