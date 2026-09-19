import { Icon } from './Icons'

export function ErrorState({ error, onRetry, title = 'We could not load this view' }) {
  return <div className="react-state react-state-error" role="alert">
    <span className="react-state-icon"><Icon name="alert" size={22}/></span>
    <strong>{title}</strong>
    <small>{error?.message || 'An unexpected error occurred.'}</small>
    {onRetry && <button type="button" className="btn btn-secondary btn-sm" onClick={onRetry}>Try again</button>}
  </div>
}
