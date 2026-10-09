import { useEffect, useMemo, useRef, useState } from 'react'
import { marked } from 'marked'

const API = '/api'

/** 当前选区是否落在节点内（用于避免刷新冲掉复制选中）。 */
export function selectionInside(el) {
  if (!el) return false
  const sel = window.getSelection?.()
  if (!sel || sel.rangeCount === 0 || sel.isCollapsed) return false
  const node = sel.anchorNode
  if (!node) return false
  return el.contains(node.nodeType === 1 ? node : node.parentNode)
}

/** 压缩多余空行，避免 marked 产出大段空白。 */
export function compactMarkdown(src) {
  return String(src || '').replace(/\r\n/g, '\n').replace(/\n{3,}/g, '\n\n')
}

/**
 * 数字员工 Markdown：鉴权拉取工作区图片，支持相对路径 / file:// / 绝对路径。
 * 选中文字时暂缓改写 innerHTML，避免流式刷新打断复制。
 */
export default function AgentMarkdown({ content, agentId, authHeaders, className = 'msg-body msg-body-flat' }) {
  const html = useMemo(
    () => marked.parse(compactMarkdown(content), { breaks: false, gfm: true }),
    [content],
  )
  const ref = useRef(null)
  const pendingHtml = useRef(html)
  const [renderHtml, setRenderHtml] = useState(html)

  useEffect(() => {
    pendingHtml.current = html
    if (selectionInside(ref.current)) return
    setRenderHtml(html)
  }, [html])

  useEffect(() => {
    const flush = () => {
      if (pendingHtml.current === renderHtml) return
      if (selectionInside(ref.current)) return
      setRenderHtml(pendingHtml.current)
    }
    document.addEventListener('selectionchange', flush)
    return () => document.removeEventListener('selectionchange', flush)
  }, [renderHtml])

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
    // authHeaders 可能每次 render 新引用；仅随正文/数字员工变化重拉
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [renderHtml, agentId])

  return <div ref={ref} className={className} dangerouslySetInnerHTML={{ __html: renderHtml }} />
}
