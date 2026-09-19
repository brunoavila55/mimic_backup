import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { api } from '../api/client'
import { ErrorState } from '../components/ErrorState'
import { Icon } from '../components/Icons'
import { LoadingState } from '../components/LoadingState'
import { Modal } from '../components/Modal'
import { formatDate, statusLabel } from '../lib/format'

function BackupContent({ backupId, mode }) {
  const query = useQuery({ queryKey: ['backup', backupId, mode], queryFn: () => mode === 'diff' ? api.backupDiff(backupId) : api.backup(backupId) })
  if (query.isLoading) return <LoadingState label={mode === 'diff' ? 'Calculating configuration diff' : 'Loading configuration'} />
  if (query.isError) return <ErrorState error={query.error} onRetry={query.refetch}/>
  if (mode === 'content') return <pre className="react-config">{query.data.config || 'No content available.'}</pre>
  return <div className="react-diff"><div className="react-diff-summary"><span>{query.data.left_version} → {query.data.right_version}</span><div><b>+{query.data.additions}</b><b>-{query.data.deletions}</b></div></div><div className="react-diff-lines">{query.data.unified_rows.map((row, index) => <div className={row.class} key={`${row.num}-${index}`}><span>{row.num}</span><b>{row.sign}</b><code>{row.line || ' '}</code></div>)}</div></div>
}

export function NodeDetailsPage() {
  const { id } = useParams()
  const [viewer, setViewer] = useState(null)
  const query = useQuery({ queryKey: ['node', id], queryFn: () => api.node(id) })
  useEffect(() => { if (query.data?.node.name) document.title = `Mimic | ${query.data.node.name}` }, [query.data])
  if (query.isLoading) return <LoadingState label="Loading node history" />
  if (query.isError) return <ErrorState error={query.error} onRetry={query.refetch} title="Node could not be loaded" />
  const { node, backups } = query.data
  return <div className="react-node-detail">
    <section className="react-page-hero"><div className="react-title-with-back"><Link to="/nodes" className="react-icon-button" aria-label="Back to nodes">←</Link><div><span className="dashboard-eyebrow">Managed device</span><h1>{node.name}</h1><p>{node.vendor} · <code>{node.ip}:{node.port}</code></p></div></div><div className="react-hero-actions"><a href={`/nodes/${node.id}/edit`} className="btn btn-primary">Edit node</a></div></section>
    <section className="react-detail-grid"><Detail label="Status"><span className={`react-status is-${node.last_status}`}>{statusLabel(node.last_status)}</span></Detail><Detail label="Last backup">{formatDate(node.last_backup_at)}</Detail><Detail label="Group">{node.group || 'Ungrouped'}</Detail><Detail label="Schedule">{node.schedule_type === 'routine' ? node.routine_name || 'Routine' : `Every ${node.frequency || 24} hours`}</Detail><Detail label="Connection"><code>{node.ip}:{node.port}</code></Detail><Detail label="State"><span className={`react-status ${node.enabled ? 'is-success' : 'is-muted'}`}>{node.enabled ? 'Enabled' : 'Paused'}</span></Detail></section>
    {node.last_error && <section className="react-error-card"><Icon name="alert"/><div><strong>Latest backup error</strong><pre>{node.last_error}</pre></div></section>}
    <section className="react-panel"><header className="react-panel-header"><div><span className="dashboard-eyebrow">Configuration archive</span><h2>Backup history</h2><p>Versioned snapshots captured from this device.</p></div><span className="react-count">{backups.length} versions</span></header>{backups.length ? <div className="react-table-scroll"><table className="react-table"><thead><tr><th>Version</th><th>Status</th><th>Changes</th><th>Date</th><th>Actions</th></tr></thead><tbody>{backups.map((backup) => <tr key={backup.id}><td><code>v{backup.version}</code></td><td><span className={`react-status is-${backup.status}`}>{backup.status}</span></td><td><span className="react-change-count"><b>+{backup.diff_additions}</b><b>-{backup.diff_deletions}</b></span></td><td>{formatDate(backup.created_at, true)}</td><td><div className="react-row-actions"><button type="button" className="btn btn-secondary btn-sm" onClick={() => setViewer({ id: backup.id, mode: 'diff', version: backup.version })}>Compare</button><button type="button" className="react-icon-button" onClick={() => setViewer({ id: backup.id, mode: 'content', version: backup.version })} aria-label={`View version ${backup.version}`}><Icon name="code"/></button></div></td></tr>)}</tbody></table></div> : <div className="react-empty"><strong>No backups performed</strong><p>The first successful snapshot will appear here.</p></div>}</section>
    <Modal open={Boolean(viewer)} onClose={() => setViewer(null)} title={viewer ? `${viewer.mode === 'diff' ? 'Compare' : 'Configuration'} · v${viewer.version}` : ''} wide>{viewer && <BackupContent backupId={viewer.id} mode={viewer.mode}/>}</Modal>
  </div>
}

function Detail({ label, children }) { return <article><span>{label}</span><strong>{children}</strong></article> }
