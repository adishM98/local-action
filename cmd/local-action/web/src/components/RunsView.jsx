import { useEffect, useState } from 'react'
import { api } from '../api.js'
import { StatusBadge } from './StatusIcon.jsx'
import RunWorkflowMenu from './RunWorkflowMenu.jsx'
import { relativeTime, duration, formatDurationMs, computeRunStats, filterRuns } from '../format.js'

const STATUS_OPTIONS = ['success', 'failed', 'running', 'queued', 'cancelled']
const STATUS_LABEL = { success: 'Passed', failed: 'Failed', running: 'Running', queued: 'Queued', cancelled: 'Cancelled' }

export default function RunsView({ repoPath, workflows, workflowFile, health, onOpenRun, onOpenSecrets }) {
  const [runs, setRuns] = useState([])
  const [error, setError] = useState(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [eventFilter, setEventFilter] = useState('')

  useEffect(() => {
    if (!repoPath) return
    let cancelled = false
    async function load() {
      try {
        const result = await api.listRuns(repoPath)
        if (!cancelled) {
          setRuns(result || [])
          setError(null)
        }
      } catch (err) {
        if (!cancelled) setError(err.message)
      }
    }
    load()
    const interval = setInterval(load, 2000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [repoPath])

  if (!repoPath) {
    return <p className="empty-state">Enter a repo path above to get started.</p>
  }

  const workflow = workflowFile ? workflows.find((w) => w.file === workflowFile) : null
  const visible = workflowFile ? runs.filter((r) => r.workflowFile === workflowFile) : runs
  const wfName = (file) => workflows.find((w) => w.file === file)?.name || file
  const events = [...new Set(visible.map((r) => r.event))].sort()
  const filtered = filterRuns(visible, { search, status: statusFilter, event: eventFilter }, (r) => wfName(r.workflowFile))

  return (
    <div className="runs-view">
      <div className="runs-view__head">
        <h2>{workflow ? workflow.name : 'All workflows'}</h2>
        {workflow && !workflow.parseError && (
          <RunWorkflowMenu
            key={workflow.file}
            repoPath={repoPath}
            workflow={workflow}
            onStarted={onOpenRun}
            onOpenSecrets={onOpenSecrets}
          />
        )}
      </div>
      {workflow?.parseError && <p className="error">{workflow.parseError}</p>}
      {health && health.dockerOK === false && (
        <div className="banner banner--warn">Docker is not running — workflow runs will fail.</div>
      )}
      {error && <p className="error">{error}</p>}
      {visible.length > 0 && <StatCards runs={visible} />}
      {visible.length > 0 && (
        <div className="toolbar">
          <input
            className="toolbar__search"
            placeholder="Search runs…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
            <option value="">All statuses</option>
            {STATUS_OPTIONS.map((s) => (
              <option key={s} value={s}>
                {STATUS_LABEL[s]}
              </option>
            ))}
          </select>
          <select value={eventFilter} onChange={(e) => setEventFilter(e.target.value)}>
            <option value="">All events</option>
            {events.map((ev) => (
              <option key={ev} value={ev}>
                {ev}
              </option>
            ))}
          </select>
        </div>
      )}
      <div className="run-rows">
        {visible.length === 0 && !error && <p className="empty-state">No runs yet.</p>}
        {visible.length > 0 && filtered.length === 0 && <p className="empty-state">No runs match these filters.</p>}
        {filtered.map((run) => (
          <button key={run.id} className="run-row" onClick={() => onOpenRun(run.id)}>
            <StatusBadge status={run.status} />
            <span className="run-row__main">
              <span className="run-row__name">
                {wfName(run.workflowFile)} #{run.id}
              </span>
              <span className="run-row__meta">
                {run.event} · {relativeTime(run.createdAt)}
              </span>
            </span>
            <span className="run-row__duration">{duration(run)}</span>
          </button>
        ))}
      </div>
    </div>
  )
}

function StatCards({ runs }) {
  const stats = computeRunStats(runs)
  return (
    <div className="stat-cards">
      <div className="stat-card">
        <span className="stat-card__value">{stats.total}</span>
        <span className="stat-card__label">Runs</span>
      </div>
      <div className="stat-card stat-card--passed">
        <span className="stat-card__value">{stats.passed}</span>
        <span className="stat-card__label">Passed</span>
      </div>
      <div className="stat-card stat-card--failed">
        <span className="stat-card__value">{stats.failed}</span>
        <span className="stat-card__label">Failed</span>
      </div>
      <div className="stat-card stat-card--running">
        <span className="stat-card__value">{stats.running}</span>
        <span className="stat-card__label">Running</span>
      </div>
      <div className="stat-card">
        <span className="stat-card__value">{stats.avgDurationMs != null ? formatDurationMs(stats.avgDurationMs) : '—'}</span>
        <span className="stat-card__label">Avg duration</span>
      </div>
    </div>
  )
}
