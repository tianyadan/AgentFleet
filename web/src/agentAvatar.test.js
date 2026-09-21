import assert from 'node:assert/strict'
import { test } from 'node:test'
import { agentAvatarUrl, DEFAULT_AGENT_AVATAR } from './agentAvatar.js'

test('空头像用默认图', () => {
  assert.equal(agentAvatarUrl(null), DEFAULT_AGENT_AVATAR)
  assert.equal(agentAvatarUrl({}), DEFAULT_AGENT_AVATAR)
  assert.equal(agentAvatarUrl({ avatar_url: '  ' }), DEFAULT_AGENT_AVATAR)
})

test('有头像用原 URL', () => {
  assert.equal(agentAvatarUrl({ avatar_url: 'https://x/a.jpg' }), 'https://x/a.jpg')
})
