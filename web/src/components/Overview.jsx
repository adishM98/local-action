import { Star, Layers, Loader2, XCircle, CheckCircle2 } from 'lucide-react'
import StatusIcon, { StatusCircle } from './StatusIcon.jsx'
import {
  computeRunStats,
  longestRunningWorkflow,
  dailyTrend,
  lastStatusByWorkflow,
  relativeTime,
  duration,
  formatDurationMs,
} from '../format.js'
import { getPinned } from '../pins.js'

const DAY_LABEL = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

export default function Overview({ repoPath, workflows, runs, onOpenRun, onNavigate }) {
  const wfName = (file) => workflows.find((w) => w.file === file)?.name || file
  const stats = computeRunStats(runs)
  const failures = runs.filter((r) => r.status === 'failed').slice(0, 5)
  const active = runs.filter((r) => r.status === 'running')
  const recent = runs.slice(0, 6)
  const longest = longestRunningWorkflow(runs, (r) => wfName(r.workflowFile))
  const trend = dailyTrend(runs, 7)
  // Refreshed on every re-render (the app already re-renders every ~2s via
  // the runs poll), so a pin toggled elsewhere shows up here without extra
  // plumbing — no need to lift pin state up through App.
  const pinnedFiles = getPinned(repoPath)
  const pinned = pinnedFiles.map((f) => workflows.find((w) => w.file === f)).filter(Boolean)
  const lastStatus = lastStatusByWorkflow(runs)

  if (!repoPath) {
    return <p className="empty-state">Enter a repo path above to get started.</p>
  }

  return (
    <div className="overview">
      <div className="overview__head">
        <h2>Overview</h2>
        <p className="runs-view__subtitle">Repository health at a glance</p>
      </div>

      <div className="overview__stats">
        <div className="overview__stat">
          <span className="overview__stat-value">{workflows.length}</span>
          <span className="overview__stat-label">Workflows</span>
        </div>
        <div className="overview__stat overview__stat--running">
          <span className="overview__stat-value">{stats.running}</span>
          <span className="overview__stat-label">Running</span>
        </div>
        <div className="overview__stat overview__stat--failed">
          <span className="overview__stat-value">{stats.failed}</span>
          <span className="overview__stat-label">Failed</span>
        </div>
        <div className="overview__stat overview__stat--passed">
          <span className="overview__stat-value">{stats.passed}</span>
          <span className="overview__stat-label">Passing</span>
        </div>
      </div>

      {runs.length === 0 ? (
        <div className="empty-state empty-state--rich">
          <p className="empty-state__heading">No workflow has run yet</p>
          <p>Pick a workflow from the sidebar and trigger a run to see activity here.</p>
        </div>
      ) : (
        <div className="overview__grid">
          {active.length > 0 && (
            <section className="overview__section">
              <h3 className="overview__section-title">
                <Loader2 size={14} className="spin" /> Running ({active.length})
              </h3>
              {active.map((run) => (
                <button className="overview__row" key={run.id} onClick={() => onOpenRun(run.id)}>
                  <StatusIcon status={run.status} />
                  <span className="overview__row-main">
                    <span className="overview__row-name">{wfName(run.workflowFile)}</span>
                    <span className="overview__row-meta">{run.branch || run.event}</span>
                  </span>
                  <span className="overview__row-time">{duration(run) || 'starting…'}</span>
                </button>
              ))}
            </section>
          )}

          {failures.length > 0 && (
            <section className="overview__section">
              <h3 className="overview__section-title">
                <XCircle size={14} /> Failures ({stats.failed})
              </h3>
              {failures.map((run) => (
                <button className="overview__row" key={run.id} onClick={() => onOpenRun(run.id)}>
                  <StatusIcon status={run.status} />
                  <span className="overview__row-main">
                    <span className="overview__row-name">{wfName(run.workflowFile)}</span>
                    <span className="overview__row-meta">{run.branch || run.event}</span>
                  </span>
                  <span className="overview__row-time">{relativeTime(run.createdAt)}</span>
                </button>
              ))}
            </section>
          )}

          <section className="overview__section">
            <h3 className="overview__section-title">
              <CheckCircle2 size={14} /> Recent runs
            </h3>
            {recent.map((run) => (
              <button className="overview__row" key={run.id} onClick={() => onOpenRun(run.id)}>
                <StatusIcon status={run.status} />
                <span className="overview__row-main">
                  <span className="overview__row-name">{wfName(run.workflowFile)}</span>
                  <span className="overview__row-meta">{run.branch || run.event}</span>
                </span>
                <span className="overview__row-time">{relativeTime(run.createdAt)}</span>
              </button>
            ))}
          </section>

          <section className="overview__section">
            <h3 className="overview__section-title">
              <Layers size={14} /> Repository health
            </h3>
            <dl className="overview__health">
              <div>
                <dt>Success rate</dt>
                <dd>{stats.total ? `${Math.round((stats.passed / stats.total) * 100)}%` : '—'}</dd>
              </div>
              <div>
                <dt>Avg. build time</dt>
                <dd>{stats.avgDurationMs != null ? formatDurationMs(stats.avgDurationMs) : '—'}</dd>
              </div>
              <div>
                <dt>Avg. queue time</dt>
                <dd>{stats.avgQueueMs != null ? formatDurationMs(stats.avgQueueMs) : '—'}</dd>
              </div>
              <div>
                <dt>Longest run</dt>
                <dd>{longest ? `${longest.name} · ${formatDurationMs(longest.durationMs)}` : '—'}</dd>
              </div>
            </dl>
            <div className="overview__trend" title="Success rate, last 7 days">
              {trend.map((day, i) => (
                <div className="overview__trend-col" key={i}>
                  <div className="overview__trend-track">
                    <div
                      className={`overview__trend-bar${day.pct == null ? ' overview__trend-bar--empty' : ''}`}
                      style={{ height: `${day.pct ?? 6}%` }}
                      title={day.pct == null ? 'No runs' : `${day.pct}% success`}
                    />
                  </div>
                  <span className="overview__trend-label">{DAY_LABEL[day.date.getDay()]}</span>
                </div>
              ))}
            </div>
          </section>

          {pinned.length > 0 && (
            <section className="overview__section">
              <h3 className="overview__section-title">
                <Star size={14} fill="currentColor" /> Pinned workflows
              </h3>
              {pinned.map((wf) => (
                <button
                  className="overview__row"
                  key={wf.file}
                  onClick={() => onNavigate({ name: 'runs', workflowFile: wf.file })}
                >
                  {lastStatus[wf.file] ? (
                    <StatusCircle status={lastStatus[wf.file]} />
                  ) : (
                    <span className="status-circle status-circle--none" />
                  )}
                  <span className="overview__row-main">
                    <span className="overview__row-name">{wf.name}</span>
                  </span>
                </button>
              ))}
            </section>
          )}
        </div>
      )}
    </div>
  )
}
