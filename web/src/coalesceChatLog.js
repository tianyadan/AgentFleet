/**
 * 结束后用 DB 校准聊天时，若库被延迟 compact 清短，保留本地更长的 assistant。
 */
export function coalesceChatLog(local = [], fromDb = []) {
  const isRealAsst = (m) =>
    m?.role === 'assistant'
    && (m.content || '').trim()
    && !(m.content || '').includes('上下文已由引擎压缩')

  const lastLocal = [...local].reverse().find(isRealAsst)
  const lastDb = [...fromDb].reverse().find(isRealAsst)
  const localLen = (lastLocal?.content || '').trim().length
  const dbLen = (lastDb?.content || '').trim().length

  // 本地流式正文更长：说明 DB 被 compact 清掉或尚未写入完整回复
  if (localLen > 0 && localLen > dbLen) {
    return local
  }
  return fromDb
}
