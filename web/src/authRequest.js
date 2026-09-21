/** 请求头里是否带了非空 Bearer 令牌。 */
export function hasBearerToken(headers) {
  const auth = headers?.Authorization || headers?.authorization || ''
  return typeof auth === 'string' && /^Bearer\s+\S+/.test(auth)
}

/**
 * 只有「带了令牌却被拒绝」才是会话过期。
 * 未登录时管理接口也会 401，不能当成过期去关掉登录弹窗。
 */
export function shouldForceLogout(status, headers) {
  return status === 401 && hasBearerToken(headers)
}
