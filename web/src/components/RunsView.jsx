import { useEffect, useRef, useState } from 'react'
import { api } from '../api.js'
import StatusIcon, { StatusCircle } from './StatusIcon.jsx'
import RunWorkflowMenu from './RunWorkflowMenu.jsx'
import { relativeTime, duration, formatDurationMs, computeRunStats, filterRuns, branchColorClass } from '../format.js'

const STATUS_OPTIONS = ['success', 'failed', 'running', 'queued', 'cancelled']
const STATUS_LABEL = { success: 'Passed', failed: 'Failed', running: 'Running', queued: 'Queued', cancelled: 'Cancelled' }
const PAGE_SIZE = 25

export default function RunsView({ repoPath, workflows, workflowFile, health, runs, runsError, onOpenRun, onOpenSecrets }) {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [eventFilter, setEventFilter] = useState('')
  const [branchFilter, setBranchFilter] = useState('')
  const [page, setPage] = useState(1)

  useEffect(() => {
    setPage(1)
  }, [workflowFile, search, statusFilter, eventFilter, branchFilter])

  if (!repoPath) {
    return <p className="empty-state">Enter a repo path above to get started.</p>
  }

  const workflow = workflowFile ? workflows.find((w) => w.file === workflowFile) : null
  const visible = workflowFile ? runs.filter((r) => r.workflowFile === workflowFile) : runs
  const wfName = (file) => workflows.find((w) => w.file === file)?.name || file
  const events = [...new Set(visible.map((r) => r.event))].sort()
  const branches = [...new Set(visible.map((r) => r.branch).filter(Boolean))].sort()
  const filtered = filterRuns(
    visible,
    { search, status: statusFilter, event: eventFilter, branch: branchFilter },
    (r) => wfName(r.workflowFile),
  )
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const pageStart = (page - 1) * PAGE_SIZE
  const paged = filtered.slice(pageStart, pageStart + PAGE_SIZE)

  return (
    <div className="runs-view">
      <div className="runs-view__head">
        <div>
          <h2>{workflow ? workflow.name : 'All workflows'}</h2>
          <p className="runs-view__subtitle">{workflow ? workflow.file : 'Every workflow run for this repo'}</p>
        </div>
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
      {runsError && <p className="error">{runsError}</p>}
      {!workflowFile && visible.length > 0 && <RecentActivity runs={visible} wfName={wfName} />}
      {visible.length > 0 && <StatCards runs={visible} />}
      {visible.length > 0 && (
        <div className="toolbar">
          <div className="toolbar__search-wrap">
            <span className="toolbar__search-icon">🔍</span>
            <input
              className="toolbar__search"
              placeholder="Filter workflow runs…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
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
          {branches.length > 0 && (
            <select value={branchFilter} onChange={(e) => setBranchFilter(e.target.value)}>
              <option value="">All branches</option>
              {branches.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
          )}
        </div>
      )}
      <div className="run-rows">
        {visible.length === 0 && !runsError && (
          <div className="empty-state empty-state--rich">
            <p className="empty-state__heading">No runs yet</p>
            <p>
              {workflow
                ? 'Trigger a run with the "Run workflow ▾" button above to see activity here.'
                : 'Pick a workflow from the sidebar and trigger a run to see activity here.'}
            </p>
          </div>
        )}
        {visible.length > 0 && filtered.length === 0 && <p className="empty-state">No runs match these filters.</p>}
        {paged.map((run) => (
          <RunRow key={run.id} run={run} name={wfName(run.workflowFile)} onOpen={() => onOpenRun(run.id)} />
        ))}
      </div>
      {filtered.length > PAGE_SIZE && (
        <div className="pagination">
          <span className="pagination__summary">
            Showing {pageStart + 1}–{Math.min(pageStart + PAGE_SIZE, filtered.length)} of {filtered.length}
          </span>
          <button className="btn" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            ← Previous
          </button>
          <span className="pagination__page">
            Page {page} of {totalPages}
          </span>
          <button className="btn" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
            Next →
          </button>
        </div>
      )}
    </div>
  )
}

function RunRow({ run, name, onOpen }) {
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef(null)
  const isTerminal = ['success', 'failed', 'cancelled'].includes(run.status)

  useEffect(() => {
    if (!menuOpen) return
    function onDocClick(e) {
      if (menuRef.current && !menuRef.current.contains(e.target)) setMenuOpen(false)
    }
    document.addEventListener('mousedown', onDocClick)
    return () => document.removeEventListener('mousedown', onDocClick)
  }, [menuOpen])

  async function quickAction(action) {
    setMenuOpen(false)
    try {
      if (action === 'cancel') await api.cancelRun(run.id)
      else if (action === 'rerun') {
        let inputs = {}
        try {
          inputs = JSON.parse(run.inputs || '{}')
        } catch {
          inputs = {}
        }
        await api.createRun({ repoPath: run.repoPath, workflowFile: run.workflowFile, event: run.event, inputs })
      }
    } catch {
      // row-level quick action — the drawer surfaces a real error message if the user opens it instead
    }
  }

  return (
    <div
      className="run-row"
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onOpen()
        }
      }}
    >
      <StatusCircle status={run.status} />
      <span className="run-row__main">
        <span className="run-row__name">
          {name} #{run.id}
        </span>
        <span className="run-row__meta">{run.event} · {relativeTime(run.createdAt)}</span>
      </span>
      {run.branch && (
        <span className={`branch-pill branch-pill--${branchColorClass(run.branch)}`}>{run.branch}</span>
      )}
      <span className="run-row__duration">{duration(run)}</span>
      <div className="run-row__overflow" ref={menuRef} onClick={(e) => e.stopPropagation()}>
        <button className="run-row__overflow-btn" onClick={() => setMenuOpen(!menuOpen)} title="More actions">
          ⋯
        </button>
        {menuOpen && (
          <div className="run-row__overflow-menu">
            <button onClick={() => { setMenuOpen(false); onOpen() }}>View details</button>
            {isTerminal ? (
              <button onClick={() => quickAction('rerun')}>Re-run</button>
            ) : (
              <button onClick={() => quickAction('cancel')}>Cancel</button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

const ACTIVITY_VERB = {
  success: 'succeeded',
  failed: 'failed',
  running: 'started',
  cancelled: 'was cancelled',
  queued: 'was queued',
}

function RecentActivity({ runs, wfName }) {
  const recent = runs.slice(0, 5)
  return (
    <div className="recent-activity">
      <div className="recent-activity__heading">Recent activity</div>
      {recent.map((run) => (
        <div className="recent-activity__item" key={run.id}>
          <StatusIcon status={run.status} />
          <span>
            {wfName(run.workflowFile)} #{run.id} {ACTIVITY_VERB[run.status] || run.status}
          </span>
          <span className="recent-activity__time">{relativeTime(run.createdAt)}</span>
        </div>
      ))}
    </div>
  )
}

function StatCards({ runs }) {
  const stats = computeRunStats(runs)
  const pct = (n) => (stats.total ? `${((n / stats.total) * 100).toFixed(1)}%` : '—')
  return (
    <div className="stat-cards">
      <div className="stat-card">
        <span className="stat-card__label">👤 Total runs</span>
        <span className="stat-card__value">{stats.total}</span>
        <span className="stat-card__sub">All time</span>
      </div>
      <div className="stat-card stat-card--passed">
        <span className="stat-card__label">✓ Successful</span>
        <span className="stat-card__value">{stats.passed}</span>
        <span className="stat-card__sub">{pct(stats.passed)}</span>
      </div>
      <div className="stat-card stat-card--failed">
        <span className="stat-card__label">✕ Failed</span>
        <span className="stat-card__value">{stats.failed}</span>
        <span className="stat-card__sub">{pct(stats.failed)}</span>
      </div>
      <div className="stat-card stat-card--running">
        <span className="stat-card__label">◔ In progress</span>
        <span className="stat-card__value">{stats.running}</span>
        <span className="stat-card__sub">{pct(stats.running)}</span>
      </div>
      <div className="stat-card">
        <span className="stat-card__label">⊘ Cancelled</span>
        <span className="stat-card__value">{stats.cancelled}</span>
        <span className="stat-card__sub">{pct(stats.cancelled)}</span>
      </div>
      <div className="stat-card">
        <span className="stat-card__label">⏱ Avg. duration</span>
        <span className="stat-card__value">{stats.avgDurationMs != null ? formatDurationMs(stats.avgDurationMs) : '—'}</span>
        <span className="stat-card__sub">&nbsp;</span>
      </div>
    </div>
  )
}
