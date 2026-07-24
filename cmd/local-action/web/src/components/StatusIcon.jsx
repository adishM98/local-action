const GLYPH = {
  success: '✓',
  failed: '✕',
  failure: '✕',
  cancelled: '⊘',
  skipped: '⊘',
  running: '●',
  queued: '○',
}

// Normalizes act's step/job results (success/failure/skipped) and run
// statuses (success/failed/cancelled/running/queued) onto one icon set.
export default function StatusIcon({ status }) {
  const s = status === 'failure' ? 'failed' : status === 'skipped' ? 'cancelled' : status || 'queued'
  return <span className={`status-icon status-icon--${s}`}>{GLYPH[status] || GLYPH[s] || '○'}</span>
}
