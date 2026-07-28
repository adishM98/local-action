const GLYPH = {
  success: '✓',
  failed: '✕',
  failure: '✕',
  cancelled: '⊘',
  skipped: '⊘',
  running: '●',
  queued: '○',
}

const LABEL = {
  success: 'Passed',
  failed: 'Failed',
  cancelled: 'Cancelled',
  running: 'Running',
  queued: 'Queued',
}

// Normalizes act's step/job results (success/failure/skipped) and run
// statuses (success/failed/cancelled/running/queued) onto one icon set.
export default function StatusIcon({ status }) {
  const s = status === 'failure' ? 'failed' : status === 'skipped' ? 'cancelled' : status || 'queued'
  return <span className={`status-icon status-icon--${s}`}>{GLYPH[status] || GLYPH[s] || '○'}</span>
}

// Pill variant for the run-detail header — a single prominent place where a
// text label earns its space. Dense contexts (job/step lists) keep the bare
// glyph, which would be too heavy as pills.
export function StatusBadge({ status }) {
  const s = status === 'failure' ? 'failed' : status === 'skipped' ? 'cancelled' : status || 'queued'
  return (
    <span className={`status-badge status-badge--${s}`}>
      <span className="status-icon">{GLYPH[status] || GLYPH[s] || '○'}</span>
      {LABEL[s] || s}
    </span>
  )
}

// Ringed-circle variant for run rows and the sidebar's per-workflow last-run
// indicator — icon only, no text label (the row/item's own name already
// carries the identity; the circle just needs to answer "did it pass?").
export function StatusCircle({ status }) {
  const s = status === 'failure' ? 'failed' : status === 'skipped' ? 'cancelled' : status || 'queued'
  return <span className={`status-circle status-circle--${s}`}>{GLYPH[status] || GLYPH[s] || '○'}</span>
}
