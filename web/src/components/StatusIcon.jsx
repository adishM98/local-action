import { CheckCircle2, XCircle, CircleSlash, CircleDashed, Loader2 } from 'lucide-react'

const ICON = {
  success: CheckCircle2,
  failed: XCircle,
  failure: XCircle,
  cancelled: CircleSlash,
  skipped: CircleSlash,
  running: Loader2,
  queued: CircleDashed,
}

const LABEL = {
  success: 'Passed',
  failed: 'Failed',
  cancelled: 'Cancelled',
  running: 'Running',
  queued: 'Queued',
}

function normalize(status) {
  return status === 'failure' ? 'failed' : status === 'skipped' ? 'cancelled' : status || 'queued'
}

function Glyph({ status, size }) {
  const s = normalize(status)
  const Icon = ICON[status] || ICON[s] || CircleDashed
  return <Icon size={size} className={s === 'running' ? 'spin' : undefined} aria-hidden="true" />
}

// Normalizes act's step/job results (success/failure/skipped) and run
// statuses (success/failed/cancelled/running/queued) onto one icon set.
export default function StatusIcon({ status }) {
  const s = normalize(status)
  return (
    <span className={`status-icon status-icon--${s}`}>
      <Glyph status={status} size={14} />
    </span>
  )
}

// Pill variant for the run-detail header — a single prominent place where a
// text label earns its space. Dense contexts (job/step lists) keep the bare
// glyph, which would be too heavy as pills.
export function StatusBadge({ status }) {
  const s = normalize(status)
  return (
    <span className={`status-badge status-badge--${s}`}>
      <span className="status-icon">
        <Glyph status={status} size={13} />
      </span>
      {LABEL[s] || s}
    </span>
  )
}

// Ringed-circle variant for run rows and the sidebar's per-workflow last-run
// indicator — icon only, no text label (the row/item's own name already
// carries the identity; the circle just needs to answer "did it pass?").
export function StatusCircle({ status }) {
  const s = normalize(status)
  return (
    <span className={`status-circle status-circle--${s}`}>
      <Glyph status={status} size={14} />
    </span>
  )
}
