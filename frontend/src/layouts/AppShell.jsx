import { useState } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { Icon } from '../components/Icons'

const titles = { '/': 'Dashboard', '/nodes': 'Nodes' }

export function AppShell() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const title = location.pathname.startsWith('/nodes/') ? 'Node details' : location.pathname.startsWith('/settings') ? 'Settings' : titles[location.pathname] || 'Mimic'
  const avatar = user.avatar || `https://ui-avatars.com/api/?name=${encodeURIComponent(user.username)}&background=171c27&color=eceee9&bold=true&size=60`

  const handleLogout = async () => {
    try { await logout() } finally { navigate('/login', { replace: true }) }
  }

  return <div className={`layout react-layout ${sidebarOpen ? 'sidebar-open' : ''}`}>
    <aside className="sidebar">
      <div className="sidebar-logo"><img src="/static/img/logo.svg" alt="Mimic"/><span className="sidebar-logo-text">Mimic</span><span className="react-version-tag">React</span></div>
      <nav className="sidebar-nav" aria-label="Primary navigation">
        <div className="sidebar-section">Main</div>
        <NavLink end to="/" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`} onClick={() => setSidebarOpen(false)}><Icon name="dashboard"/><span>Dashboard</span></NavLink>
        <NavLink to="/nodes" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`} onClick={() => setSidebarOpen(false)}><Icon name="nodes"/><span>Nodes</span></NavLink>
        <div className="sidebar-section">System</div>
        <NavLink to="/settings" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`} onClick={() => setSidebarOpen(false)}><Icon name="settings"/><span>Settings</span></NavLink>
      </nav>
      <div className="sidebar-user">
        <div className="sidebar-user-inner"><div className="user-avatar"><img src={avatar} alt=""/></div><div className="user-info"><div className="user-name">{user.username}</div><div className="user-role">{user.role}</div></div></div>
        <button type="button" className="sidebar-logout" onClick={handleLogout}><Icon name="logout"/><span>Log out</span></button>
        <div className="react-sidebar-foot">Mimic Backup v0.8.1</div>
      </div>
    </aside>
    <button type="button" className="sidebar-overlay" aria-label="Close menu" onClick={() => setSidebarOpen(false)}/>
    <main className="main">
      <header className="header"><div className="header-left"><button type="button" className="mobile-menu-btn" onClick={() => setSidebarOpen(true)}><Icon name="menu"/><span>Menu</span></button><div className="breadcrumb"><span className="breadcrumb-root">Mimic</span><span className="breadcrumb-sep">/</span><span className="breadcrumb-current">{title}</span></div></div><div className="header-right"><span className="react-live"><i/>Session secured</span></div></header>
      <div className="content page-enter"><Outlet /></div>
      <footer className="app-footer">© 2026 Mimic Backup Systems v0.8.1</footer>
    </main>
  </div>
}
