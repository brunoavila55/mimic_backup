import { createContext, useCallback, useContext, useMemo, useState } from 'react'

const ToastContext = createContext(null)

export function ToastProvider({ children }) {
  const [items, setItems] = useState([])
  const notify = useCallback((message, type = 'success') => {
    const id = crypto.randomUUID()
    setItems((current) => [...current, { id, message, type }])
    window.setTimeout(() => setItems((current) => current.filter((item) => item.id !== id)), 4200)
  }, [])
  const value = useMemo(() => ({ notify }), [notify])
  return <ToastContext.Provider value={value}>
    {children}
    <div className="react-toast-region" aria-live="polite">
      {items.map((item) => <div className={`notification is-${item.type}`} key={item.id}>
        <div><div className="notif-label">{item.type}</div><div className="notif-message">{item.message}</div></div>
        <button type="button" onClick={() => setItems((current) => current.filter((entry) => entry.id !== item.id))} aria-label="Dismiss">×</button>
      </div>)}
    </div>
  </ToastContext.Provider>
}

export function useToast() {
  const context = useContext(ToastContext)
  if (!context) throw new Error('useToast must be used inside ToastProvider')
  return context
}
