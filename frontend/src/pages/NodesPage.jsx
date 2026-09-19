import { useEffect, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import { ErrorState } from '../components/ErrorState'
import { Icon } from '../components/Icons'
import { LoadingState } from '../components/LoadingState'
import { formatDate, statusLabel } from '../lib/format'

export function NodesPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const filters = Object.fromEntries(searchParams)
  const [search, setSearch] = useState(filters.search || '')
  useEffect(() => { document.title = 'Mimic | Nodes' }, [])
  useEffect(() => {
    const timer = window.setTimeout(() => {
      const next = new URLSearchParams(searchParams)
      if (search) next.set('search', search); else next.delete('search')
      if (next.toString() !== searchParams.toString()) setSearchParams(next, { replace: true })
    }, 350)
    return () => window.clearTimeout(timer)
  }, [search, searchParams, setSearchParams])
  const query = useQuery({ queryKey: ['nodes', filters], queryFn: () => api.nodes(filters), placeholderData: keepPreviousData })
  const updateFilter = (key, value) => {
    const next = new URLSearchParams(searchParams)
    if (value) next.set(key, value); else next.delete(key)
    setSearchParams(next)
  }
  if (query.isLoading) return <LoadingState label="Loading network inventory" />
  if (query.isError) return <ErrorState error={query.error} onRetry={query.refetch} />
  const { nodes, stats, groups } = query.data
  return <div className="react-nodes">
    <section className="react-page-hero"><div><span className="dashboard-eyebrow"><Icon name="nodes" size={14}/>Network inventory</span><h1>Nodes</h1><p>Manage devices, backup health, schedules and maintenance actions in one place.</p></div><div className="react-hero-actions"><a href="/nodes/export" className="btn btn-secondary">Export CSV</a><a href="/nodes/import" className="btn btn-secondary">Import CSV</a><a href="/nodes/new" className="btn btn-primary">+ New node</a></div></section>
    <section className="react-node-stats"><Metric label="Total nodes" value={stats.total}/><Metric label="Active" value={stats.active} tone="success"/><Metric label="Needs attention" value={stats.attention} tone="danger"/><Metric label="Muted alerts" value={stats.muted} tone="muted"/></section>
    <section className="react-node-toolbar"><label className="react-search"><Icon name="search"/><input type="search" className="form-input" placeholder="Search by name, IP, vendor, group or tag…" value={search} onChange={(event) => setSearch(event.target.value)}/></label><div><select className="form-input" value={filters.status || ''} onChange={(event) => updateFilter('status', event.target.value)}><option value="">All status</option><option value="active">Active only</option><option value="inactive">Inactive</option><option value="success">Healthy</option><option value="error">With errors</option><option value="pending">Pending</option><option value="muted">Muted alerts</option></select><select className="form-input" value={filters.vendor || ''} onChange={(event) => updateFilter('vendor', event.target.value)}><option value="">All vendors</option>{['cisco','mikrotik','huawei','juniper'].map((vendor) => <option value={vendor} key={vendor}>{vendor[0].toUpperCase() + vendor.slice(1)}</option>)}</select><select className="form-input" value={filters.group || ''} onChange={(event) => updateFilter('group', event.target.value)}><option value="">All groups</option>{groups.map((group) => <option value={group} key={group}>{group}</option>)}</select>{searchParams.size > 0 && <button className="btn btn-ghost btn-sm" type="button" onClick={() => { setSearch(''); setSearchParams({}) }}>Clear</button>}</div></section>
    <div className={`react-table-wrap ${query.isFetching ? 'is-updating' : ''}`}><table className="react-table"><thead><tr><th>Node</th><th>Connection</th><th>Schedule</th><th>Health</th><th>Last backup</th><th>State</th><th aria-label="Actions"/></tr></thead><tbody>{nodes.map((node) => <tr key={node.id} className={node.last_status === 'error' ? 'is-error' : ''}><td><div className="react-node-title"><span><Icon name="nodes"/></span><div><Link to={`/nodes/${node.id}`}>{node.name}</Link><small>{node.group || 'Ungrouped'}{node.tags ? ` · ${node.tags}` : ''}</small></div></div></td><td><code>{node.ip}:{node.port}</code><small className="react-vendor">{node.vendor}</small></td><td><strong>{node.schedule_type === 'routine' ? 'Routine' : 'Individual'}</strong><small>{node.schedule_type === 'routine' ? node.routine_name || 'Not selected' : `Every ${node.frequency || 24}h${node.backup_hour ? ` at ${node.backup_hour}` : ''}`}</small></td><td><span className={`react-status is-${node.last_status || 'pending'}`}>{statusLabel(node.last_status)}</span></td><td><span>{formatDate(node.last_backup_at)}</span><small>{node.next_backup_at ? `Next ${formatDate(node.next_backup_at)}` : 'Not scheduled'}</small></td><td><span className={`react-status ${node.enabled ? 'is-success' : 'is-muted'}`}>{node.enabled ? 'Enabled' : 'Paused'}</span></td><td><Link to={`/nodes/${node.id}`} className="react-icon-button" aria-label={`Open ${node.name}`}><Icon name="arrow"/></Link></td></tr>)}</tbody></table>{!nodes.length && <div className="react-empty react-table-empty"><Icon name="nodes" size={28}/><strong>No nodes found</strong><p>Adjust the filters or create the first node in this inventory.</p></div>}</div>
  </div>
}

function Metric({ label, value, tone = '' }) { return <article className={tone ? `is-${tone}` : ''}><span>{label}</span><strong>{value}</strong></article> }
