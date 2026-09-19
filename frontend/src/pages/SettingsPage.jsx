import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Navigate, NavLink, useParams } from 'react-router-dom'
import { api } from '../api/client'
import { ErrorState } from '../components/ErrorState'
import { Icon } from '../components/Icons'
import { LoadingState } from '../components/LoadingState'
import { useAuth } from '../hooks/useAuth'
import { formatDate } from '../lib/format'

const definitions = [
  { id: 'profile', label: 'My profile', permission: null },
  { id: 'credentials', label: 'Credentials', permission: 'manage_operations' },
  { id: 'routines', label: 'Routines', permission: 'manage_operations' },
  { id: 'users', label: 'Users', permission: 'manage_users' },
  { id: 'logs', label: 'Audit logs', permission: 'view_audit' },
  { id: 'alerts', label: 'Alerts', permission: 'manage_system' },
  { id: 'sftp', label: 'SFTP', permission: 'manage_system' },
  { id: 'export', label: 'Export', permission: 'export_backups' },
]

const actions = {
  profile: { href: '/settings/profile?legacy=1', label: 'Edit profile' },
  credentials: { href: '/settings/credentials/new', label: 'New credential' },
  routines: { href: '/settings/routines/new', label: 'New routine' },
  users: { href: '/settings/users/new', label: 'New user' },
  alerts: { href: '/settings/alerts/new', label: 'New rule' },
  sftp: { href: '/settings/sftp?legacy=1', label: 'Edit connection' },
  export: { href: '/settings/export?legacy=1', label: 'Manage exports' },
}

export function SettingsPage() {
  const { tab } = useParams()
  const { user } = useAuth()
  const available = definitions.filter((item) => !item.permission || user.permissions.includes(item.permission))
  const current = available.find((item) => item.id === tab)
  const query = useQuery({ queryKey: ['settings', tab], queryFn: () => api.settings(tab), enabled: Boolean(current) })
  useEffect(() => { document.title = `Mimic | ${current?.label || 'Settings'}` }, [current])
  if (!current) return <Navigate to={`/settings/${available[0]?.id || 'profile'}`} replace />

  return <div className="react-settings">
    <section className="react-page-hero"><div><span className="dashboard-eyebrow"><Icon name="settings" size={14}/>Workspace control</span><h1>Settings</h1><p>Manage access, automation, delivery and your personal workspace.</p></div>{actions[tab] && <a className="btn btn-primary" href={actions[tab].href}>{actions[tab].label}</a>}</section>
    <nav className="react-settings-tabs" aria-label="Settings sections">{available.map((item) => <NavLink key={item.id} to={`/settings/${item.id}`} className={({ isActive }) => isActive ? 'active' : ''}>{item.label}</NavLink>)}</nav>
    <section className="react-settings-content">
      {query.isLoading && <LoadingState label={`Loading ${current.label.toLowerCase()}`} />}
      {query.isError && (
        <ErrorState error={query.error} onRetry={query.refetch}/>
      )}
      {query.data && (
        <SettingsContent tab={tab} data={query.data} currentUser={user}/>
      )}
    </section>
  </div>
}

function SettingsContent({ tab, data, currentUser }) {
  if (tab === 'profile') return <div className="react-profile-card"><img src={data.avatar || `https://ui-avatars.com/api/?name=${encodeURIComponent(data.username)}&background=171c27&color=eceee9&bold=true&size=120`} alt=""/><div><span className="dashboard-eyebrow">Signed-in identity</span><h2>{data.username}</h2><p>{data.email || 'No email configured'}</p><span className="react-status is-success">{data.role}</span><small>Member since {formatDate(data.created_at)}</small></div></div>
  if (tab === 'credentials') return <CardGrid items={data.credentials} empty="No credentials configured">{(item) => <SettingCard key={item.id} eyebrow={`SSH · port ${item.port}`} title={item.name} subtitle={item.username || 'No username'} meta={`${item.node_count} linked nodes`} state={item.has_password ? 'Secured' : 'Password missing'} tone={item.has_password ? 'success' : 'warning'} href={`/settings/credentials/${item.id}/edit`}/>}</CardGrid>
  if (tab === 'routines') return <CardGrid items={data.routines} empty="No backup routines configured">{(item) => <SettingCard key={item.id} eyebrow={`Every ${item.frequency}h · ${item.backup_hour || 'time not set'}`} title={item.name} subtitle={item.description || 'No description'} meta={`${item.node_count} linked nodes`} state={item.enabled ? 'Active' : 'Paused'} tone={item.enabled ? 'success' : 'muted'} href={`/settings/routines/${item.id}/edit`}/>}</CardGrid>
  if (tab === 'users') return <CardGrid items={data.users} empty="No users found">{(item) => <SettingCard key={item.id} eyebrow={item.id === data.current_user_id ? 'Current session' : 'Workspace member'} title={item.username} subtitle={item.email || 'No email'} meta={`Created ${formatDate(item.created_at)}`} state={item.role} tone={item.role === 'Administrator' ? 'success' : 'muted'} href={`/settings/users/${item.id}/edit`}/>}</CardGrid>
  if (tab === 'alerts') return <CardGrid items={data.alerts} empty="No alert rules configured">{(item) => <SettingCard key={item.id} eyebrow={`${item.provider} · ${item.target_group}`} title={item.name} subtitle={[item.alert_on_failure && 'Failures', item.alert_on_diff && 'Config changes'].filter(Boolean).join(' + ') || 'No events selected'} meta={item.configured ? 'Destination configured' : 'Destination missing'} state={item.enabled ? 'Enabled' : 'Disabled'} tone={item.enabled && item.configured ? 'success' : 'warning'} href={`/settings/alerts/${item.id}/edit`}/>}</CardGrid>
  if (tab === 'logs') return <div className="react-log-list">{data.logs.map((entry) => <article key={entry.id}><i className={`is-${entry.level}`}/><div><span><b>{entry.category}</b><time>{formatDate(entry.created_at, true)}</time></span><strong>{entry.message}</strong>{entry.details && <code>{entry.details}</code>}</div></article>)}</div>
  if (tab === 'sftp') return <div className="react-connection-card"><div className={`react-connection-icon ${data.configured ? 'is-success' : ''}`}><Icon name={data.configured ? 'check' : 'alert'} size={25}/></div><div><span className="dashboard-eyebrow">Remote delivery</span><h2>{data.configured ? `${data.host}:${data.port}` : 'SFTP is not configured'}</h2><p>{data.configured ? `${data.username} · ${data.path || '/'}` : 'Add a remote destination to export configuration snapshots.'}</p><div><span className={`react-status ${data.enabled ? 'is-success' : 'is-muted'}`}>{data.enabled ? 'Automatic sync enabled' : 'Automatic sync paused'}</span><span className="react-status">Last result: {data.last_export_status || 'never'}</span></div>{data.last_export_at && <small>Last export {formatDate(data.last_export_at)}</small>}</div></div>
  if (tab === 'export') return <div className="react-table-wrap standalone"><table className="react-table"><thead><tr><th>Node</th><th>Connection</th><th>Group</th><th>Latest backup</th><th>Export state</th></tr></thead><tbody>{data.nodes.map((node) => <tr key={node.id}><td><strong>{node.name}</strong><small>{node.vendor}</small></td><td><code>{node.ip}</code></td><td>{node.group}</td><td>{formatDate(node.backup_at)}</td><td><span className={`react-status is-${node.export_state === 'exported' ? 'success' : node.export_state === 'pending' ? 'pending' : 'muted'}`}>{node.export_state}</span></td></tr>)}</tbody></table></div>
  return <p>{currentUser.username}</p>
}

function CardGrid({ items, empty, children }) {
  if (!items.length) return <div className="react-empty"><strong>{empty}</strong></div>
  return <div className="react-setting-grid">{items.map(children)}</div>
}

function SettingCard({ eyebrow, title, subtitle, meta, state, tone, href }) {
  return <a className="react-setting-card" href={href}><div><span>{eyebrow}</span><h2>{title}</h2><p>{subtitle}</p></div><footer><small>{meta}</small><span className={`react-status is-${tone}`}>{state}</span></footer></a>
}
