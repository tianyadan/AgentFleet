/** 管理台菜单用的白色描边图标（24 视口，现代线性风格）。 */
export function AdminMenuIcon({ name }) {
  return (
    <svg className="admin-menu-icon" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {iconBody(name)}
    </svg>
  )
}

/** 数字员工列表向左收起 / 展开。 */
export function ListCollapseIcon({ collapsed }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M9 3v18" />
      {collapsed ? <path d="m14 9 3 3-3 3" /> : <path d="m16 15-3-3 3-3" />}
    </svg>
  )
}

function iconBody(name) {
  switch (name) {
    case 'history':
      return (
        <>
          <path d="M3 12a9 9 0 1 0 3-6.7L3 8" />
          <path d="M3 3v5h5" />
          <path d="M12 7v5l3 2" />
        </>
      )
    case 'stats':
      return (
        <>
          <path d="M3 3v18h18" />
          <path d="M18 17V9" />
          <path d="M13 17V5" />
          <path d="M8 17v-3" />
        </>
      )
    case 'agents':
      return (
        <>
          <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
          <circle cx="9" cy="7" r="4" />
          <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
          <path d="M16 3.13a4 4 0 0 1 0 7.75" />
        </>
      )
    case 'plans':
      return (
        <>
          <rect x="3" y="3" width="7" height="7" rx="1.5" />
          <rect x="14" y="14" width="7" height="7" rx="1.5" />
          <path d="M6.5 10v3a2 2 0 0 0 2 2H14" />
          <path d="M17.5 14v-1.5a2 2 0 0 0-2-2H10" />
        </>
      )
    case 'memos':
      return (
        <>
          <path d="M15 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z" />
          <path d="M15 3v5h5" />
          <path d="M8 13h8" />
          <path d="M8 17h5" />
        </>
      )
    case 'passwords':
      return (
        <>
          <circle cx="8" cy="15" r="5" />
          <path d="m12 11 9-9" />
          <path d="M16 6l3 3" />
          <path d="M18 4l2 2" />
        </>
      )
    case 'servers':
      return (
        <>
          <rect x="3" y="3" width="18" height="7" rx="2" />
          <rect x="3" y="14" width="18" height="7" rx="2" />
          <path d="M7 6.5h.01" />
          <path d="M7 17.5h.01" />
        </>
      )
    case 'analytics':
      return (
        <>
          <path d="M3 3v18h18" />
          <path d="m7 14 4-4 3 3 5-6" />
        </>
      )
    case 'changelog':
      return (
        <>
          <path d="M12 7v6l3 2" />
          <path d="M4 12a8 8 0 1 0 2.3-5.6" />
          <path d="M4 4v4h4" />
        </>
      )
    default:
      return <circle cx="12" cy="12" r="8" />
  }
}
