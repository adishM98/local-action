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
  let cancelled = 0
  let finishedCount = 0
  let finishedTotalMs = 0

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
  }

  return {
    total: runs.length,
    passed,
    failed,
    running,
    cancelled,
    avgDurationMs: finishedCount ? Math.round(finishedTotalMs / finishedCount) : null,
  }
}

const BRANCH_COLORS = ['blue', 'purple', 'green', 'orange', 'pink']

// branchColorClass deterministically maps a branch name to one of a small
// fixed palette (same branch always renders the same color; different
// branches usually land on different ones) — a CSS class suffix for
// .branch-pill--<color>. No color for an empty branch (nothing to render).
export function branchColorClass(branch) {
  if (!branch) return ''
  let hash = 0
  for (let i = 0; i < branch.length; i++) {
    hash = (hash * 31 + branch.charCodeAt(i)) | 0
  }
  return BRANCH_COLORS[Math.abs(hash) % BRANCH_COLORS.length]
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
