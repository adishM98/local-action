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
// avgQueueMs is the average wait between a run being created and actually
// starting (only over runs that have started — a still-queued run's wait
// isn't over yet and would understate the average).
export function computeRunStats(runs) {
  let passed = 0
  let failed = 0
  let running = 0
  let cancelled = 0
  let finishedCount = 0
  let finishedTotalMs = 0
  let queuedCount = 0
  let queuedTotalMs = 0

  for (const run of runs) {
    if (run.status === 'success') passed++
    else if (run.status === 'failed') failed++
    else if (run.status === 'running') running++
    else if (run.status === 'cancelled') cancelled++

    const start = unwrap(run.startedAt)
    const end = unwrap(run.finishedAt)
    if (start != null && end != null) {
      finishedCount++
      finishedTotalMs += (end - start) * 1000
    }
    if (start != null && run.createdAt != null) {
      queuedCount++
      queuedTotalMs += Math.max(0, start - run.createdAt) * 1000
    }
  }

  return {
    total: runs.length,
    passed,
    failed,
    running,
    cancelled,
    avgDurationMs: finishedCount ? Math.round(finishedTotalMs / finishedCount) : null,
    avgQueueMs: queuedCount ? Math.round(queuedTotalMs / queuedCount) : null,
  }
}

// longestRunningWorkflow returns the single longest-running finished run
// (by wall-clock duration), with its display name resolved via resolveName,
// or null if nothing has finished yet.
export function longestRunningWorkflow(runs, resolveName) {
  let best = null
  for (const run of runs) {
    const start = unwrap(run.startedAt)
    const end = unwrap(run.finishedAt)
    if (start == null || end == null) continue
    const durationMs = (end - start) * 1000
    if (!best || durationMs > best.durationMs) {
      best = { name: resolveName(run), durationMs }
    }
  }
  return best
}

const DAY_MS = 24 * 60 * 60 * 1000

// dailyTrend buckets runs into the last `days` calendar days (local time,
// oldest first) and computes each day's success rate over its terminal
// (success/failed) runs. A day with no terminal runs gets pct: null so the
// caller can render it as an empty bar instead of a misleading 0%.
export function dailyTrend(runs, days = 7) {
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const buckets = Array.from({ length: days }, (_, i) => {
    const date = new Date(today.getTime() - (days - 1 - i) * DAY_MS)
    return { date, passed: 0, failed: 0 }
  })

  for (const run of runs) {
    if (run.status !== 'success' && run.status !== 'failed') continue
    const created = new Date(run.createdAt * 1000)
    created.setHours(0, 0, 0, 0)
    const bucket = buckets.find((b) => b.date.getTime() === created.getTime())
    if (!bucket) continue
    if (run.status === 'success') bucket.passed++
    else bucket.failed++
  }

  return buckets.map((b) => {
    const total = b.passed + b.failed
    return { date: b.date, pct: total ? Math.round((b.passed / total) * 100) : null }
  })
}

// lastRunByWorkflow maps each workflow file to its most recent run object.
// Assumes runs is already newest-first (ListRuns orders by created_at
// DESC), so the first occurrence per file wins. A workflow with no entry
// has never been run.
export function lastRunByWorkflow(runs) {
  const byFile = {}
  for (const run of runs) {
    if (!(run.workflowFile in byFile)) byFile[run.workflowFile] = run
  }
  return byFile
}

// lastStatusByWorkflow maps each workflow file to the status of its most
// recent run.
export function lastStatusByWorkflow(runs) {
  const byFile = lastRunByWorkflow(runs)
  const statuses = {}
  for (const file in byFile) statuses[file] = byFile[file].status
  return statuses
}

// filterRuns applies the runs-list toolbar filters. search matches
// workflow name / event / "#<id>" / status, case-insensitively; status and
// event narrow further (empty string = no restriction). resolveName maps a
// run's workflowFile to its display name (workflows aren't embedded in Run).
export function filterRuns(runs, { search, status, event, branch }, resolveName) {
  const q = search.trim().toLowerCase()
  return runs.filter((run) => {
    if (status && run.status !== status) return false
    if (event && run.event !== event) return false
    if (branch && run.branch !== branch) return false
    if (!q) return true
    const haystack = `${resolveName(run)} ${run.event} #${run.id} ${run.status}`.toLowerCase()
    return haystack.includes(q)
  })
}

export function repoNameFor(path) {
  return path ? path.split('/').filter(Boolean).pop() || path : ''
}
