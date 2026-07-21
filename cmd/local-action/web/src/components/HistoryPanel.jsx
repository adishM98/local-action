import { useEffect, useState } from 'react'
import { api } from '../api.js'
import LogViewer from './LogViewer.jsx'

const STATUS_DOT = {
  success: 'ok',
  failed: 'bad',
  cancelled: 'bad',
  running: 'running',
  queued: 'queued',
}

function StatusChip({ status }) {
  return (
    <span className="status-chip">
      <span className={`dot dot--${STATUS_DOT[status] || 'queued'}`} />
      {status}
    </span>
  )
}

export default function HistoryPanel({ repoPath, activeRunId }) {
  const [runs, setRuns] = useState([])
  const [selectedId, setSelectedId] = useState(activeRunId)

  async function load() {
    if (!repoPath) return
    setRuns(await api.listRuns(repoPath))
  }

  useEffect(() => {
    load()
    const interval = setInterval(load, 2000)
    return () => clearInterval(interval)
  }, [repoPath])

  useEffect(() => {
    if (activeRunId) setSelectedId(activeRunId)
  }, [activeRunId])

  if (!repoPath) {
    return <p className="empty-state">Enter a repo path above to see its run history.</p>
  }

  return (
    <div className="row row--top">
      <ul className="run-list">
        {runs.length === 0 && <p className="empty-state">No runs yet for this repo.</p>}
        {runs.map((run) => (
          <li
            key={run.id}
            className={run.id === selectedId ? 'selected' : ''}
            onClick={() => setSelectedId(run.id)}
          >
            <span className="run-list__title">
              #{run.id} {run.workflowFile}
            </span>
            <StatusChip status={run.status} />
          </li>
        ))}
      </ul>
      {selectedId ? (
        <LogViewer runId={selectedId} onCancel={() => api.cancelRun(selectedId)} />
      ) : (
        <p className="empty-state">Select a run to view its log.</p>
      )}
    </div>
  )
}
