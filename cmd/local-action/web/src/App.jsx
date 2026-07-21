import { useState } from 'react'
import PromptBar from './components/PromptBar.jsx'
import WorkflowsPanel from './components/WorkflowsPanel.jsx'
import SecretsPanel from './components/SecretsPanel.jsx'
import HistoryPanel from './components/HistoryPanel.jsx'

const TABS = [
  { id: 'workflows', label: 'Workflows', glyph: '▤' },
  { id: 'secrets', label: 'Secrets', glyph: '◈' },
  { id: 'history', label: 'History', glyph: '◷' },
]

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
      <PromptBar repoPath={repoPath} setRepoPath={updateRepoPath} />
      <div className="shell">
        <nav className="side-nav">
          {TABS.map((t) => (
            <button
              key={t.id}
              className={`side-nav__item${tab === t.id ? ' active' : ''}`}
              onClick={() => setTab(t.id)}
            >
              <span className="side-nav__glyph">{t.glyph}</span>
              {t.label}
            </button>
          ))}
        </nav>
        <div className="content">
          {tab === 'workflows' && (
            <WorkflowsPanel repoPath={repoPath} onRunStarted={onRunStarted} />
          )}
          {tab === 'secrets' && <SecretsPanel repoPath={repoPath} />}
          {tab === 'history' && <HistoryPanel repoPath={repoPath} activeRunId={activeRunId} />}
        </div>
      </div>
    </div>
  )
}
