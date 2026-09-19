import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import { ErrorState } from '../components/ErrorState'
import { Icon } from '../components/Icons'
import { LoadingState } from '../components/LoadingState'
import { formatDate, formatTime } from '../lib/format'

function Metric({ label, value, detail, tone = '' }) {
  return <article className={`react-metric ${tone ? `is-${tone}` : ''}`}><span>{label}</span><strong>{value}</strong><small>{detail}</small></article>
}

export function DashboardPage() {
  const query = useQuery({ queryKey: ['dashboard'], queryFn: api.dashboard, refetchInterval: 45_000 })
  useEffect(() => { document.title = 'Mimic | Dashboard' }, [])
  if (query.isLoading) return <LoadingState label="Building operations snapshot" />
  if (query.isError) return <ErrorState error={query.error} onRetry={query.refetch} />

  const data = query.data
  const stats = data.stats
  const hasNodes = stats.total_nodes > 0
  return <div className="react-dashboard">
    <section className="react-page-hero">
      <div><span className="dashboard-eyebrow"><span className="dashboard-live-dot"/>Live operations</span><h1>Network backup control center</h1><p>Prioritize risk, verify coverage and keep configuration recovery ready.</p></div>
      <div className="react-hero-actions"><div className="react-updated"><span>Updated</span><strong>{formatTime(data.updated_at)}</strong></div><button className="btn btn-secondary" type="button" onClick={() => query.refetch()} disabled={query.isFetching}><Icon name="refresh" className={query.isFetching ? 'is-spinning' : ''}/>{query.isFetching ? 'Refreshing' : 'Refresh'}</button><Link to="/nodes" className="btn btn-primary">Review fleet</Link></div>
    </section>

    {!hasNodes ? <section className="react-onboarding"><div><span className="dashboard-eyebrow">Workspace setup</span><h2>Build your first protected network</h2><p>Add access credentials, register a node and attach a backup routine. Mimic will start building configuration history automatically.</p><a href="/nodes/new" className="btn btn-primary">Add first node</a></div><ol><li><b>01</b><span><strong>Secure access</strong><small>Store reusable SSH credentials.</small></span></li><li><b>02</b><span><strong>Add inventory</strong><small>Register routers, switches and firewalls.</small></span></li><li><b>03</b><span><strong>Automate recovery</strong><small>Assign a routine and verify the snapshot.</small></span></li></ol></section> : <>
      <section className="react-fleet-summary">
        <article className={`react-readiness ${stats.attention_nodes ? 'is-warning' : 'is-healthy'}`}><div className="react-progress-ring" style={{ '--progress': `${stats.health_rate}%` }}><span><strong>{stats.health_rate}%</strong><small>healthy</small></span></div><div><span className="dashboard-eyebrow">Fleet readiness</span><h2>{stats.attention_nodes ? `${stats.attention_nodes} nodes require action` : 'Recovery coverage is stable'}</h2><p>{stats.failed_nodes} failed and {stats.silent_nodes} without a recent successful backup.</p><a href="#attention">Review action queue</a></div></article>
        <div className="react-metrics"><Metric label="Active fleet" value={stats.active_nodes} detail={`of ${stats.total_nodes} registered`}/><Metric label="Success · 24h" value={stats.backups_24h ? `${stats.success_rate_24h}%` : '—'} detail={`${stats.successful_24h} of ${stats.backups_24h} runs`} tone={stats.failed_24h ? 'warning' : 'success'}/><Metric label="Needs attention" value={stats.attention_nodes} detail="failed or stale nodes" tone={stats.attention_nodes ? 'danger' : ''}/><Metric label="Config changes · 24h" value={stats.changes_24h} detail="successful runs with drift" tone="info"/></div>
      </section>

      <section className="react-dashboard-grid">
        <div className="react-dashboard-main">
          <article id="attention" className="react-panel"><header className="react-panel-header"><div><span className="dashboard-eyebrow">Priority queue</span><h2>Action required</h2><p>Failures and stale coverage, ordered by urgency.</p></div><span className={`react-count ${stats.attention_nodes ? 'is-danger' : 'is-success'}`}>{stats.attention_nodes} open</span></header>{data.attention.length ? <div className="react-list">{data.attention.map((item) => <Link className={`react-attention is-${item.severity}`} to={`/nodes/${item.node.id}`} key={item.node.id}><i/><div><div><strong>{item.node.name}</strong><span>{item.label}</span></div><b>{item.title}</b><p>{item.detail}</p><small>{item.node.ip} · {item.context}</small></div><Icon name="arrow"/></Link>)}</div> : <div className="react-empty-positive"><span><Icon name="check"/></span><div><strong>Queue is clear</strong><p>No failed or stale nodes require action.</p></div></div>}</article>
          <article className="react-panel"><header className="react-panel-header"><div><span className="dashboard-eyebrow">Reliability</span><h2>Backup outcomes · 7 days</h2><p>Successful and failed executions by day.</p></div><div className="react-legend"><span>Success</span><span>Failed</span></div></header><div className="react-chart">{data.trend.map((day) => <div className="react-chart-day" key={day.date} title={`${day.successful} successful, ${day.failed} failed`}><b>{day.total}</b><div><i className="success" style={{ height: `${day.success_height}%` }}/><i className="failed" style={{ height: `${day.failure_height}%` }}/></div><strong>{day.label}</strong><small>{day.date}</small></div>)}</div><footer className="react-panel-footer"><span>{stats.backups_24h} executions in the last 24 hours</span><span>Latest success {formatDate(data.last_successful_at)}</span></footer></article>
        </div>
        <aside className="react-dashboard-side">
          <article className="react-panel"><header className="react-panel-header"><div><span className="dashboard-eyebrow">Controls</span><h2>Coverage & risk</h2><p>Readiness across the active fleet.</p></div></header><div className="react-coverage"><div><span><b>Recent backup coverage</b><strong>{stats.healthy_nodes}/{stats.active_nodes}</strong></span><progress value={stats.health_rate} max="100"/><small>{stats.health_rate}% backed up within 48h</small></div><div><span><b>Schedule coverage</b><strong>{stats.scheduled_nodes}/{stats.active_nodes}</strong></span><progress value={stats.schedule_coverage} max="100"/><small>{stats.schedule_coverage}% have a next execution</small></div></div><div className="react-risk"><span>EX</span><div><strong>{stats.pending_export_nodes} pending exports</strong><small>{data.sftp.detail}</small></div><b className={`is-${data.sftp.tone}`}>{data.sftp.state}</b></div></article>
          <article className="react-panel"><header className="react-panel-header"><div><span className="dashboard-eyebrow">Schedule</span><h2>Next executions</h2><p>Upcoming automated backup window.</p></div></header><div className="react-list">{data.upcoming.length ? data.upcoming.map((node) => <Link className="react-upcoming" to={`/nodes/${node.id}`} key={node.id}><time><strong>{new Date(node.next_backup_at).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'})}</strong><small>{new Date(node.next_backup_at).toLocaleDateString([], {day: '2-digit', month: 'short'})}</small></time><div><strong>{node.name}</strong><small>{node.vendor} · {node.ip}</small></div><span>{node.group}</span></Link>) : <div className="react-empty"><strong>No upcoming executions</strong><p>Assign a routine or individual schedule.</p></div>}</div></article>
        </aside>
      </section>

      <section className="react-intelligence"><article className="react-panel"><header className="react-panel-header"><div><span className="dashboard-eyebrow">Configuration intelligence</span><h2>Recent changes</h2><p>Latest successful backups with detected drift.</p></div></header><div className="react-list">{data.recent_changes.length ? data.recent_changes.map((backup) => <Link to={`/nodes/${backup.node_id}`} className="react-change" key={backup.id}><div><strong>{backup.node_name}</strong><small>v{backup.version} · {formatDate(backup.created_at)}</small></div><span><b>+{backup.diff_additions}</b><b>-{backup.diff_deletions}</b></span><Icon name="arrow"/></Link>) : <div className="react-empty"><strong>No recent configuration drift</strong><p>Changes will appear after successful backups.</p></div>}</div></article><article className="react-panel"><header className="react-panel-header"><div><span className="dashboard-eyebrow">Audit trail</span><h2>Recent activity</h2><p>Latest operational events.</p></div></header><div className="react-list">{data.recent_activity.map((entry) => <div className="react-activity" key={entry.id}><i className={`is-${entry.level}`}/><div><strong>{entry.message}</strong><small>{entry.category} · {formatDate(entry.created_at)}</small></div></div>)}</div></article></section>
    </>}
  </div>
}
