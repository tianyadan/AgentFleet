import { coalesceChatLog } from './coalesceChatLog.js'

function assert(cond, msg) {
  if (!cond) throw new Error(msg)
}

{
  const local = [
    { role: 'user', content: 'hi' },
    { role: 'assistant', content: '· full answer here that is long enough' },
  ]
  const db = [
    { role: 'assistant', content: '📦 上下文已由引擎压缩，会话已保留' },
    { role: 'user', content: 'hi' },
  ]
  const got = coalesceChatLog(local, db)
  assert(got === local, 'should keep local when db lost assistant')
}

{
  const local = [{ role: 'user', content: 'q' }, { role: 'assistant', content: 'short' }]
  const db = [
    { role: 'user', content: 'q' },
    { role: 'assistant', content: 'short but from db with more detail xyz' },
  ]
  const got = coalesceChatLog(local, db)
  assert(got === db, 'prefer longer db')
}

console.log('coalesceChatLog ok')
