import { useEffect, useState } from 'react'
import { Star, Layers, Loader2, XCircle, CheckCircle2, AlertTriangle, Package, GitBranch, FolderOpen, Clock } from 'lucide-react'
import StatusIcon, { StatusCircle } from './StatusIcon.jsx'
import {
  computeRunStats,
  longestRunningWorkflow,
  dailyTrend,
  lastStatusByWorkflow,
  relativeTime,
  duration,
  formatDurationMs,
  repoNameFor,
} from '../format.js'
import { getPinned } from '../pins.js'

const DAY_LABEL = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const RECENT_KEY = 'recentRepoPaths'
const MAX_RECENT = 8

function rememberPath(path) {
  if (!path) return
  const existing = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
  const next = [path, ...existing.filter((p) => p !== path)].slice(0, MAX_RECENT)
  localStorage.setItem(RECENT_KEY, JSON.stringify(next))
}

export default function Overview({ repoPath, onCommit, branch, workflows, runs, onOpenRun, onNavigate }) {
  const [editing, setEditing] = useState(!repoPath)
  const [draft, setDraft] = useState(repoPath)
  const [showRecent, setShowRecent] = useState(false)
  const recentPaths = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')

  useEffect(() => {
    setDraft(repoPath)
    setEditing(!repoPath)
  }, [repoPath])

  useEffect(() => {
    if (!showRecent) return
    function onClickOutside(e) {
      if (!e.target.closest('.repo-header__recent-wrap')) setShowRecent(false)
    }
    document.addEventListener('mousedown', onClickOutside)
    return () => document.removeEventListener('mousedown', onClickOutside)
  }, [showRecent])

  function commitPath() {
    const path = draft.trim()
    // Nothing to commit — stay in editing mode instead of stranding a
    // first-time user on a screen with no visible input and no repo name.
    if (!path) return
    setEditing(false)
    if (path !== repoPath) {
      rememberPath(path)
      onCommit(path)
    }
  }

  // window.pickRepoFolder is bound natively by the DMG app (cmd/local-action-gui)
  // and opens a real macOS folder picker — a regular browser tab has no way
  // to expose a real filesystem path to JS at all, so the button simply
  // doesn't render there instead of pretending to work.
  const hasNativeFolderPicker = typeof window.pickRepoFolder === 'function'

  async function browseFolder() {
    let path
    try {
      path = await window.pickRepoFolder()
    } catch (err) {
      // A failure here would otherwise vanish silently (no visible error,
      // no popup) — log it so it's at least diagnosable from the app's
      // console/log file instead of just looking like nothing happened.
      console.error('pickRepoFolder failed:', err)
      return
    }
    if (!path) return
    setDraft(path)
    setEditing(false)
    if (path !== repoPath) {
      rememberPath(path)
      onCommit(path)
    }
  }

  // The browser-side equivalent of browseFolder — no real filesystem
  // access is possible from a browser tab, so this picks from paths
  // already used before rather than pretending to browse the disk.
  function pickRecent(path) {
    setShowRecent(false)
    setDraft(path)
    setEditing(false)
    if (path !== repoPath) {
      rememberPath(path)
      onCommit(path)
    }
  }

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
  const incompatible = workflows.filter((w) => w.incompatibleRunners?.length > 0)

  return (
    <div className="overview">
      <div className="overview__head">
        <div className="overview__identity">
          <span className="overview__identity-icon">
            <Package size={28} />
          </span>
          {editing ? (
            <div className="repo-header__path-wrap overview__identity-input">
              <input
                className="repo-header__path-input"
                list="recent-repo-paths"
                autoFocus
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onBlur={commitPath}
                onKeyDown={(e) => e.key === 'Enter' && commitPath()}
                placeholder="/path/to/repo"
                spellCheck={false}
              />
              <datalist id="recent-repo-paths">
                {recentPaths.map((p) => (
                  <option key={p} value={p} />
                ))}
              </datalist>
              {hasNativeFolderPicker ? (
                <button
                  type="button"
                  className="repo-header__browse"
                  // Clicking this button would otherwise blur the input
                  // first, and commitPath's blur handler unconditionally
                  // exits editing mode — collapsing this whole block
                  // before the click (and the folder pick it starts)
                  // even runs. Preventing the mousedown default stops the
                  // focus shift, so no blur fires at all.
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={browseFolder}
                  title="Browse for a folder"
                >
                  <FolderOpen size={15} />
                </button>
              ) : (
                recentPaths.length > 0 && (
                  <div className="repo-header__recent-wrap">
                    <button
                      type="button"
                      className="repo-header__browse"
                      onMouseDown={(e) => e.preventDefault()}
                      onClick={() => setShowRecent((s) => !s)}
                      title="Recent repo paths"
                    >
                      <Clock size={15} />
                    </button>
                    {showRecent && (
                      <div className="repo-header__recent-menu">
                        {recentPaths.map((p) => (
                          <button
                            key={p}
                            type="button"
                            onMouseDown={(e) => e.preventDefault()}
                            onClick={() => pickRecent(p)}
                          >
                            {p}
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                )
              )}
            </div>
          ) : (
            <div className="overview__identity-text">
              <div className="overview__identity-top">
                <h2>{repoNameFor(repoPath)}</h2>
                {/* A folder icon, not a pencil — the action is "point
                    this at a different local repo", not "edit metadata".
                    Sits right next to the name it acts on, not floating
                    off in the card's far corner. */}
                <button
                  className="repo-header__browse repo-header__browse--inline"
                  onClick={() => setEditing(true)}
                  title="Change repository"
                >
                  <FolderOpen size={13} />
                </button>
              </div>
              {/* Plain text now, not a button — the action that used to
                  live here (enter edit mode) moved to the button above.
                  Branch is metadata to read, not something you click. */}
              <span className="repo-header__meta" title={repoPath}>
                {branch ? (
                  <>
                    <span className="repo-header__meta-icon">
                      <GitBranch size={13} strokeWidth={2.25} />
                    </span>
                    {branch}
                  </>
                ) : (
                  <span className="repo-header__meta-path">{repoPath}</span>
                )}
              </span>
            </div>
          )}
        </div>
        {repoPath && !editing && (
          <p className="overview__meta-line">
            {workflows.length} workflow{workflows.length === 1 ? '' : 's'} ·{' '}
            <span className={stats.running > 0 ? 'overview__meta-count--active' : undefined}>
              {stats.running} running
            </span>{' '}
            ·{' '}
            <span className={stats.failed > 0 ? 'overview__meta-count--bad' : undefined}>
              {stats.failed} failed
            </span>
          </p>
        )}
      </div>

      {!repoPath ? (
        <p className="empty-state">Enter a repo path above to get started.</p>
      ) : (
        <>
          {incompatible.length > 0 && (
            <div className="overview__card overview__incompatible-card">
              <h3 className="overview__section-title">
                <AlertTriangle size={14} /> Runner compatibility ({incompatible.length})
              </h3>
              <section className="overview__section">
                {incompatible.map((wf) => (
                  <button
                    className="overview__row"
                    key={wf.file}
                    onClick={() => onNavigate({ name: 'runs', workflowFile: wf.file })}
                    title={`act only emulates Linux — this may fail or behave differently than real CI`}
                  >
                    <AlertTriangle size={14} className="overview__row-warn-icon" />
                    <span className="overview__row-main">
                      <span className="overview__row-name">{wf.name}</span>
                      <span className="overview__row-meta">
                        runs-on <code>{wf.incompatibleRunners.join(', ')}</code>
                      </span>
                    </span>
                  </button>
                ))}
              </section>
            </div>
          )}

          {runs.length === 0 ? (
            <div className="empty-state empty-state--rich">
              <p className="empty-state__heading">No workflow has run yet</p>
              {workflows.length === 0 ? (
                <p>
                  Add a workflow file under <code>.github/workflows</code>, then rescan the repo to get started.
                </p>
              ) : (
                <>
                  <ol className="empty-state__steps">
                    <li>Pick a workflow from the Explorer in the sidebar</li>
                    <li>Choose an event to simulate</li>
                    <li>Click Run</li>
                  </ol>
                  {workflows.length === 1 && (
                    <button
                      className="btn btn--primary empty-state__cta"
                      onClick={() => onNavigate({ name: 'runs', workflowFile: workflows[0].file })}
                    >
                      Open {workflows[0].name}
                    </button>
                  )}
                </>
              )}
            </div>
          ) : (
            <div className="overview__grid">
              {active.length > 0 && (
                <div className="overview__card">
                  <h3 className="overview__section-title">
                    <Loader2 size={14} className="spin" /> Running ({active.length})
                  </h3>
                  <section className="overview__section">
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
                </div>
              )}
    
              {failures.length > 0 && (
                <div className="overview__card">
                  <h3 className="overview__section-title">
                    <XCircle size={14} /> Failures ({stats.failed})
                  </h3>
                  <section className="overview__section">
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
                </div>
              )}
    
              <div className="overview__card">
                <h3 className="overview__section-title">
                  <CheckCircle2 size={14} /> Recent activity
                </h3>
                <section className="overview__section">
                  {recent.map((run) => (
                    <button className="overview__row" key={run.id} onClick={() => onOpenRun(run.id)}>
                      <StatusIcon status={run.status} />
                      <span className="overview__row-name">{wfName(run.workflowFile)}</span>
                      <span className="overview__row-time">{relativeTime(run.createdAt)}</span>
                    </button>
                  ))}
                </section>
              </div>
    
              <div className="overview__card overview__section--wide">
                <h3 className="overview__section-title">
                  <Layers size={14} /> Repository health
                </h3>
                <section className="overview__section">
                  {stats.total > 0 && (
                    <div className="overview__success">
                      <div className="overview__success-bar">
                        <div
                          className="overview__success-fill"
                          style={{ width: `${Math.round((stats.passed / stats.total) * 100)}%` }}
                        />
                      </div>
                      <span className="overview__success-pct">{Math.round((stats.passed / stats.total) * 100)}%</span>
                    </div>
                  )}
                  <dl className="overview__health">
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
              </div>
    
              {pinned.length > 0 && (
                <div className="overview__card">
                  <h3 className="overview__section-title">
                    <Star size={14} fill="currentColor" /> Pinned workflows
                  </h3>
                  <section className="overview__section">
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
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}
