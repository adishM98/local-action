import { useState } from 'react'
import { StatusCircle } from './StatusIcon.jsx'

export default function Sidebar({ workflows, scanState, view, onNavigate, runs }) {
  const [search, setSearch] = useState('')
  const inRuns = view.name === 'runs'

  const q = search.trim().toLowerCase()
  const matching = q ? workflows.filter((wf) => wf.name.toLowerCase().includes(q)) : workflows

  // First (most recent — ListRuns orders newest-first) run per workflow.
  const lastStatus = {}
  for (const run of runs) {
    if (!(run.workflowFile in lastStatus)) lastStatus[run.workflowFile] = run.status
  }

  return (
    <nav className="sidebar">
      <div className="sidebar__heading">Actions</div>
      {workflows.length > 5 && (
        <input
          className="sidebar__search"
          placeholder="Search workflows…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      )}
      <button
        className={`sidebar__item sidebar__item--flat${inRuns && !view.workflowFile ? ' active' : ''}`}
        onClick={() => onNavigate({ name: 'runs', workflowFile: null })}
      >
        All workflows
      </button>
      {q && matching.length === 0 && <p className="sidebar__note">No workflows match "{search}".</p>}
      {matching.map((wf) => (
        <button
          key={wf.file}
          className={`sidebar__item sidebar__item--flat${inRuns && view.workflowFile === wf.file ? ' active' : ''}`}
          onClick={() => onNavigate({ name: 'runs', workflowFile: wf.file })}
          title={wf.file}
        >
          {lastStatus[wf.file] ? <StatusCircle status={lastStatus[wf.file]} /> : <span className="status-circle status-circle--none" />}
          <span className="sidebar__item-label">{wf.name}</span>
        </button>
      ))}
      {scanState.error && <p className="sidebar__note sidebar__note--error">{scanState.error}</p>}
      {scanState.scanned && !scanState.error && workflows.length === 0 && (
        <p className="sidebar__note">No workflows under .github/workflows.</p>
      )}
      <div className="sidebar__spacer" />
      <div className="sidebar__heading">Settings</div>
      <button
        className={`sidebar__item${view.name === 'secrets' ? ' active' : ''}`}
        onClick={() => onNavigate({ name: 'secrets', workflowFile: null })}
      >
        Secrets and variables
      </button>
    </nav>
  )
}
