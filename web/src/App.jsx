import { useState } from 'react'
import WorkflowsPanel from './components/WorkflowsPanel.jsx'

export default function App() {
  const [repoPath, setRepoPath] = useState(localStorage.getItem('repoPath') || '')
  const [tab, setTab] = useState('workflows')
  const [activeRunId, setActiveRunId] = useState(null)

  function updateRepoPath(path) {
    setRepoPath(path)
    localStorage.setItem('repoPath', path)
  }

  function onRunStarted(runId) {
    setActiveRunId(runId)
    setTab('history')
  }

  return (
    <div className="app">
      <nav>
        <button onClick={() => setTab('workflows')}>Workflows</button>
        <button onClick={() => setTab('secrets')}>Secrets</button>
        <button onClick={() => setTab('history')}>History</button>
      </nav>
      {tab === 'workflows' && (
        <WorkflowsPanel repoPath={repoPath} setRepoPath={updateRepoPath} onRunStarted={onRunStarted} />
      )}
      {tab === 'secrets' && <p>Secrets panel placeholder — added in Task 11.</p>}
      {tab === 'history' && <p>History panel placeholder — added in Task 12.</p>}
    </div>
  )
}
