export default function Sidebar({ workflows, scanState, view, onNavigate }) {
  const inRuns = view.name === 'runs' || view.name === 'run'
  return (
    <nav className="sidebar">
      <div className="sidebar__heading">Actions</div>
      <button
        className={`sidebar__item${inRuns && !view.workflowFile ? ' active' : ''}`}
        onClick={() => onNavigate({ name: 'runs', workflowFile: null })}
      >
        All workflows
      </button>
      {workflows.map((wf) => (
        <button
          key={wf.file}
          className={`sidebar__item${inRuns && view.workflowFile === wf.file ? ' active' : ''}`}
          onClick={() => onNavigate({ name: 'runs', workflowFile: wf.file })}
          title={wf.file}
        >
          {wf.name}
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
