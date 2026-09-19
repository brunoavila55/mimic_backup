export function formatDate(value, withSeconds = false) {
  if (!value) return '—'
  return new Intl.DateTimeFormat('en-GB', {
    day: '2-digit', month: 'short', year: 'numeric',
    hour: '2-digit', minute: '2-digit', ...(withSeconds ? { second: '2-digit' } : {}),
  }).format(new Date(value))
}

export function formatTime(value) {
  if (!value) return '—'
  return new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}

export function statusLabel(status) {
  return ({ success: 'Healthy', error: 'Error', never: 'Pending', pending: 'Pending' })[status] || 'Pending'
}
