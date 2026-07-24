import { useEffect, useState } from 'react'
import { api } from '../api.js'
import StatusIcon from './StatusIcon.jsx'
import RunWorkflowMenu from './RunWorkflowMenu.jsx'
import { relativeTime, duration } from '../format.js'

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
      <div className="run-rows">
        {visible.length === 0 && !error && <p className="empty-state">No runs yet.</p>}
        {visible.map((run) => (
          <button key={run.id} className="run-row" onClick={() => onOpenRun(run.id)}>
            <StatusIcon status={run.status} />
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
