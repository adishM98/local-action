function unwrap(nullable) {
  if (nullable == null) return null
  if (typeof nullable === 'number') return nullable
  return nullable.Valid ? nullable.Int64 : null
}

export function relativeTime(unixSeconds) {
  if (!unixSeconds) return ''
  const delta = Math.floor(Date.now() / 1000) - unixSeconds
  if (delta < 60) return 'just now'
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`
  return `${Math.floor(delta / 86400)}d ago`
}

export function formatDurationMs(ms) {
  if (ms == null) return ''
  const totalSeconds = Math.round(ms / 1000)
  const m = Math.floor(totalSeconds / 60)
  const s = totalSeconds % 60
  return m ? `${m}m ${s}s` : `${s}s`
}

export function duration(run) {
  const start = unwrap(run.startedAt)
  if (!start) return ''
  const end = unwrap(run.finishedAt) ?? Math.floor(Date.now() / 1000)
  const secs = Math.max(0, end - start)
  const m = Math.floor(secs / 60)
  const s = secs % 60
  return m ? `${m}m ${s}s` : `${s}s`
}

// computeRunStats aggregates the dashboard stat-card numbers from an
// already-loaded runs list — no separate API call. avgDurationMs is only
// computed over runs with both startedAt and finishedAt set (a run still
// in progress has no finishedAt yet and would skew the average).
export function computeRunStats(runs) {
  let passed = 0
  let failed = 0
  let running = 0
  let finishedCount = 0
  let finishedTotalMs = 0

  for (const run of runs) {
    if (run.status === 'success') passed++
    else if (run.status === 'failed') failed++
    else if (run.status === 'running') running++

    const start = unwrap(run.startedAt)
    const end = unwrap(run.finishedAt)
    if (start != null && end != null) {
      finishedCount++
      finishedTotalMs += (end - start) * 1000
    }
  }

  return {
    total: runs.length,
    passed,
    failed,
    running,
    avgDurationMs: finishedCount ? Math.round(finishedTotalMs / finishedCount) : null,
  }
}
