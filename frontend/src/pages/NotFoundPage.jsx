import { Link } from 'react-router-dom'

export function NotFoundPage() {
  return <div className="react-not-found"><span className="dashboard-eyebrow">404 · Route not found</span><h1>This panel does not exist</h1><p>The requested view may still belong to the legacy interface or was moved.</p><Link to="/" className="btn btn-primary">Return to dashboard</Link></div>
}
