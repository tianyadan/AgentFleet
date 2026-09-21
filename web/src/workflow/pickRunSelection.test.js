import assert from 'node:assert/strict'
import { test } from 'node:test'
import { nextRunSelection } from './pickRunSelection.js'

const execs = [
  { id: 1, node_id: 'a', status: 'success', attempt: 1 },
  { id: 2, node_id: 'b', status: 'running', attempt: 1 },
]

test('未手动选择时跟随 running', () => {
  const got = nextRunSelection(execs, { followLive: true, curId: 1, nodeId: 'a' })
  assert.deepEqual(got, { id: 2, nodeId: 'b' })
})

test('手动选择后保住当前节点', () => {
  const got = nextRunSelection(execs, { followLive: false, curId: 1, nodeId: 'a' })
  assert.deepEqual(got, { id: 1, nodeId: 'a' })
})

test('手动选择的执行消失时仍绑定该节点最新 attempt', () => {
  const later = [...execs, { id: 3, node_id: 'a', status: 'success', attempt: 2 }]
  const got = nextRunSelection(later, { followLive: false, curId: 99, nodeId: 'a' })
  assert.deepEqual(got, { id: 3, nodeId: 'a' })
})
