import { useEffect, useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'

export function LoginPage() {
  const [form, setForm] = useState({ username: '', password: '' })
  const [error, setError] = useState('')
  const { isAuthenticated, isLoading, login, loginPending } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const destination = location.state?.from?.pathname || '/'

  useEffect(() => { document.title = 'Mimic | Login' }, [])
  if (!isLoading && isAuthenticated) return <Navigate to={destination} replace />

  const submit = async (event) => {
    event.preventDefault()
    setError('')
    try {
      await login(form)
      navigate(destination, { replace: true })
    } catch (requestError) {
      setError(requestError.message)
    }
  }

  return <main className="react-login">
    <section className="react-login-brand">
      <div className="react-login-brand-inner">
        <div className="react-login-logo"><img src="/static/img/logo.svg" alt="Mimic"/><span>Mimic</span></div>
        <span className="dashboard-eyebrow"><span className="dashboard-live-dot"/>Configuration intelligence</span>
        <h1>Know what changed.<br/><em>Recover with confidence.</em></h1>
        <p>Continuous network configuration backups, versioned history and audit-ready visibility in one control plane.</p>
        <div className="react-login-proof"><div><strong>SHA-256</strong><span>Integrity tracking</span></div><div><strong>AES-GCM</strong><span>Secret storage</span></div><div><strong>RBAC</strong><span>Scoped access</span></div></div>
      </div>
    </section>
    <section className="react-login-panel">
      <form className="react-login-form" onSubmit={submit}>
        <div><span className="dashboard-eyebrow">Secure workspace</span><h2>Welcome back</h2><p>Sign in to open the operations console.</p></div>
        {error && <div className="react-form-error" role="alert">{error}</div>}
        <label className="form-group"><span>Username</span><input className="form-input" name="username" autoComplete="username" autoFocus required value={form.username} onChange={(event) => setForm({ ...form, username: event.target.value })}/></label>
        <label className="form-group"><span>Password</span><input className="form-input" type="password" name="password" autoComplete="current-password" required value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })}/></label>
        <button type="submit" className="btn btn-primary react-login-submit" disabled={loginPending}>{loginPending ? 'Authenticating…' : 'Open control center'}</button>
        <small className="react-login-note">Session cookies remain encrypted, HttpOnly and same-origin.</small>
      </form>
    </section>
  </main>
}
