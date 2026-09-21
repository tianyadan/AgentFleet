/** 项目协作节点工具栏 SVG 图标（无 emoji） */

const svgProps = {
  width: 14,
  height: 14,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 2,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
  'aria-hidden': true,
}

export function IconPlay() {
  return (
    <svg {...svgProps}>
      <polygon points="6 4 20 12 6 20 6 4" fill="currentColor" stroke="none" />
    </svg>
  )
}

export function IconBot() {
  return (
    <svg {...svgProps}>
      <rect x="4" y="8" width="16" height="12" rx="2" />
      <path d="M12 8V4" />
      <circle cx="9" cy="14" r="1" fill="currentColor" stroke="none" />
      <circle cx="15" cy="14" r="1" fill="currentColor" stroke="none" />
    </svg>
  )
}

export function IconUserCheck() {
  return (
    <svg {...svgProps}>
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M16 11l2 2 4-4" />
    </svg>
  )
}

export function IconBranch() {
  return (
    <svg {...svgProps}>
      <path d="M6 3v12" />
      <circle cx="18" cy="6" r="3" />
      <circle cx="6" cy="18" r="3" />
      <path d="M18 9a9 9 0 0 1-9 9" />
    </svg>
  )
}

export function IconParallel() {
  return (
    <svg {...svgProps}>
      <path d="M8 4v16" />
      <path d="M16 4v16" />
      <path d="M4 8h4" />
      <path d="M16 8h4" />
      <path d="M4 16h4" />
      <path d="M16 16h4" />
    </svg>
  )
}

export function IconMerge() {
  return (
    <svg {...svgProps}>
      <path d="M8 4v8a4 4 0 0 0 4 4h0a4 4 0 0 0 4-4V4" />
      <path d="M12 16v4" />
    </svg>
  )
}

export function IconStop() {
  return (
    <svg {...svgProps}>
      <rect x="5" y="5" width="14" height="14" rx="2" fill="currentColor" stroke="none" />
    </svg>
  )
}

export function IconSettings() {
  return (
    <svg {...svgProps}>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 1v2M12 21v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M1 12h2M21 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4" />
    </svg>
  )
}

export function IconFolder() {
  return (
    <svg {...svgProps}>
      <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
    </svg>
  )
}

/** 编辑铅笔 */
export function IconEdit() {
  return (
    <svg {...svgProps}>
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
    </svg>
  )
}

/** 删除/垃圾桶 */
export function IconTrash() {
  return (
    <svg {...svgProps}>
      <path d="M3 6h18" />
      <path d="M8 6V4h8v2" />
      <path d="M19 6l-1 14H6L5 6" />
      <path d="M10 11v6M14 11v6" />
    </svg>
  )
}

/** 查看任务状态 */
export function IconActivity() {
  return (
    <svg {...svgProps}>
      <path d="M22 12h-4l-3 7-4-14-3 7H2" />
    </svg>
  )
}

export const NODE_TYPE_ICONS = {
  start: IconPlay,
  agent: IconBot,
  human_review: IconUserCheck,
  condition: IconBranch,
  parallel: IconParallel,
  merge: IconMerge,
  end: IconStop,
}
