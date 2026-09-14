import { useEffect, useMemo, useRef } from 'react'
import { marked } from 'marked'

const API = '/api'

/**
 * 智能体 Markdown：鉴权拉取工作区图片，支持相对路径 / file:// / 绝对路径。
 */
export default function AgentMarkdown({ content, agentId, authHeaders, className = 'msg-body msg-body-flat' }) {
  const html = useMemo(() => marked.parse(content || ''), [content])
  const ref = useRef(null)

  useEffect(() => {
    const root = ref.current
    if (!root || !agentId) return undefined
    const imgs = [...root.querySelectorAll('img')]
    const revoke = []
    let cancelled = false

    imgs.forEach((img) => {
      const src = (img.getAttribute('src') || '').trim()
      if (!src || src.startsWith('blob:') || src.startsWith('data:')) return
      // 远程 http(s) 直接展示；本地路径走鉴权 API
      if (/^https?:\/\//i.test(src)) return

      const path = src.replace(/^file:\/\//i, '')
      ;(async () => {
        try {
          const headers = typeof authHeaders === 'function' ? authHeaders() : {}
          const url = `${API}/admin/agents/${agentId}/file?path=${encodeURIComponent(path)}`
          const res = await fetch(url, { headers })
          if (!res.ok || cancelled) return
          const blob = await res.blob()
          if (cancelled) return
          const blobUrl = URL.createObjectURL(blob)
          revoke.push(blobUrl)
          img.src = blobUrl
          img.classList.add('ma-md-img')
        } catch {
          /* ignore broken image */
        }
      })()
    })

    return () => {
      cancelled = true
      revoke.forEach((u) => URL.revokeObjectURL(u))
    }
    // authHeaders 可能每次 render 新引用；仅随正文/智能体变化重拉
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [html, agentId])

  return <div ref={ref} className={className} dangerouslySetInnerHTML={{ __html: html }} />
}
