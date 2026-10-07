import { useState } from 'react'
import { marked } from 'marked'
import { PRODUCT_MANUAL_CATEGORIES, PRODUCT_MANUAL_ITEMS } from './productManual.js'

/** 后台产品手册：静态 Markdown 内容按分类展示，无需后端接口。 */
export default function ProductManual() {
  const [activeId, setActiveId] = useState(PRODUCT_MANUAL_CATEGORIES[0]?.itemIds?.[0] || '')
  const active = PRODUCT_MANUAL_ITEMS.find((item) => item.id === activeId) || PRODUCT_MANUAL_ITEMS[0]

  if (!active) return null

  return (
    <section className="product-manual">
      <aside className="product-manual-tree" aria-label="产品手册分类">
        <p className="product-manual-tree-title">产品手册</p>
        {PRODUCT_MANUAL_CATEGORIES.map((category) => (
          <div key={category.id} className="product-manual-category">
            <p>{category.label}</p>
            {(category.itemIds || []).map((id) => {
              const item = PRODUCT_MANUAL_ITEMS.find((candidate) => candidate.id === id)
              if (!item) return null
              return (
                <button
                  key={item.id}
                  type="button"
                  className={item.id === active.id ? 'active' : ''}
                  onClick={() => setActiveId(item.id)}
                >
                  <span>{item.label}</span>
                  <small>{item.description}</small>
                </button>
              )
            })}
          </div>
        ))}
      </aside>
      <article
        className="product-manual-content"
        dangerouslySetInnerHTML={{ __html: marked.parse(active.content) }}
      />
    </section>
  )
}
