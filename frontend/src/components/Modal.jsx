import { useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { Icon } from './Icons'

export function Modal({ open, onClose, title, children, wide = false }) {
  const panelRef = useRef(null)
  useEffect(() => {
    if (!open) return undefined
    const previous = document.activeElement
    const onKeyDown = (event) => { if (event.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKeyDown)
    document.body.style.overflow = 'hidden'
    panelRef.current?.focus()
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      document.body.style.overflow = ''
      previous?.focus?.()
    }
  }, [open, onClose])
  if (!open) return null
  return createPortal(<div className="modal-overlay react-modal-overlay" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <section ref={panelRef} className={`modal-box react-modal ${wide ? 'is-wide' : ''}`} role="dialog" aria-modal="true" aria-label={title} tabIndex={-1}>
      <header className="react-modal-header"><div><span className="dashboard-eyebrow">Configuration archive</span><h2>{title}</h2></div><button type="button" className="react-icon-button" onClick={onClose} aria-label="Close"><Icon name="close"/></button></header>
      <div className="react-modal-content">{children}</div>
    </section>
  </div>, document.body)
}
