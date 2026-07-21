import { useEffect, useState } from 'react'
import { api } from '../api.js'
import LogViewer from './LogViewer.jsx'

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

  return (
    <div className="row">
      <ul className="run-list">
        {runs.map((run) => (
          <li key={run.id} onClick={() => setSelectedId(run.id)}>
            #{run.id} {run.workflowFile} [{run.event}] — {run.status}
          </li>
        ))}
      </ul>
      {selectedId && <LogViewer runId={selectedId} onCancel={() => api.cancelRun(selectedId)} />}
    </div>
  )
}
