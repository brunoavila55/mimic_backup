export function LoadingState({ label = 'Loading data' }) {
  return <div className="react-state" role="status"><span className="react-spinner"/><strong>{label}</strong><small>Please wait a moment.</small></div>
}

export function LoadingScreen({ label }) {
  return <main className="react-screen-center"><img src="/static/img/logo.svg" alt=""/><LoadingState label={label} /></main>
}
