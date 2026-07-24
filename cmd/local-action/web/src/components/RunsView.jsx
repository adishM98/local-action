import { useEffect, useState } from 'react'
import { api } from '../api.js'
import { StatusBadge } from './StatusIcon.jsx'
import RunWorkflowMenu from './RunWorkflowMenu.jsx'
import { relativeTime, duration, formatDurationMs, computeRunStats } from '../format.js'

export default function RunsView({ repoPath, workflows, workflowFile, health, onOpenRun, onOpenSecrets }) {
  const [runs, setRuns] = useState([])
  const [error, setError] = useState(null)

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
      <div className="run-rows">
        {visible.length === 0 && !error && <p className="empty-state">No runs yet.</p>}
        {visible.map((run) => (
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
