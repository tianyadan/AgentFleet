import assert from 'node:assert/strict'
import { test } from 'node:test'
import { hasBearerToken, shouldForceLogout } from './authRequest.js'

test('未带令牌不是已登录请求', () => {
  assert.equal(hasBearerToken(undefined), false)
  assert.equal(hasBearerToken({}), false)
  assert.equal(hasBearerToken({ Authorization: '' }), false)
  assert.equal(hasBearerToken({ Authorization: 'Bearer ' }), false)
})

test('带非空 Bearer 才算已登录请求', () => {
  assert.equal(hasBearerToken({ Authorization: 'Bearer abc' }), true)
  assert.equal(hasBearerToken({ authorization: 'Bearer abc' }), true)
})

test('未登录的 401 不能强制退出', () => {
  assert.equal(shouldForceLogout(401, {}), false)
  assert.equal(shouldForceLogout(401, undefined), false)
})

test('带令牌的 401 才是会话过期', () => {
  assert.equal(shouldForceLogout(401, { Authorization: 'Bearer abc' }), true)
  assert.equal(shouldForceLogout(200, { Authorization: 'Bearer abc' }), false)
  assert.equal(shouldForceLogout(403, { Authorization: 'Bearer abc' }), false)
})
