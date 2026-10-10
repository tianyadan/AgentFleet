/** 数字员工默认头像（百度图床固定 URL） */
export const DEFAULT_AGENT_AVATAR =
  'https://img0.baidu.com/it/u=890210585,2769039861&fm=253&fmt=auto&app=138&f=JPEG?w=500&h=667'

/** 历史错误 CNAME（证书主机名不匹配，浏览器无法加载）→ 官方 OSS 公网域名 */
const BROKEN_AVATAR_HOST = 'digital-employee-qd.cn-qingdao.taihangcda.cn'
const OFFICIAL_AVATAR_HOST = 'digital-employee-qd.oss-cn-qingdao.aliyuncs.com'

/** 纠正不可访问的历史头像域名 */
export function normalizeAvatarUrl(url) {
  const u = String(url || '').trim()
  if (!u) return ''
  if (u.includes(BROKEN_AVATAR_HOST)) {
    return u.split(BROKEN_AVATAR_HOST).join(OFFICIAL_AVATAR_HOST)
  }
  return u
}

/** 解析展示用头像 URL；空则默认图 */
export function agentAvatarUrl(a) {
  const u = normalizeAvatarUrl(a?.avatar_url)
  return u || DEFAULT_AGENT_AVATAR
}

/**
 * 前端预压缩：最长边 ≤ maxEdge，输出 JPEG Blob。
 * @param {File} file
 * @param {number} [maxEdge=512]
 */
export function compressImageFile(file, maxEdge = 512) {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      URL.revokeObjectURL(url)
      let { width: w, height: h } = img
      if (w < 1 || h < 1) {
        reject(new Error('invalid image'))
        return
      }
      if (w >= h && w > maxEdge) {
        h = Math.round((h * maxEdge) / w)
        w = maxEdge
      } else if (h > maxEdge) {
        w = Math.round((w * maxEdge) / h)
        h = maxEdge
      }
      const canvas = document.createElement('canvas')
      canvas.width = w
      canvas.height = h
      const ctx = canvas.getContext('2d')
      ctx.drawImage(img, 0, 0, w, h)
      canvas.toBlob(
        (blob) => {
          if (!blob) reject(new Error('compress failed'))
          else resolve(blob)
        },
        'image/jpeg',
        0.82,
      )
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('load image failed'))
    }
    img.src = url
  })
}
