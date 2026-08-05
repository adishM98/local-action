import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import { api } from '../api.js'
import JobGraph from './JobGraph.jsx'
import WorkflowSource from './WorkflowSource.jsx'

const NO_RUNTIME_JOBS = new Map()
function noop() {}

// Same Graph/Code panes as the run drawer, minus Logs and any live-status
// join — there's no run to color nodes from or highlight a failure in, just
// the workflow's static shape. Lets "No runs yet" workflows still answer
// "what does this actually do" before anyone triggers one.
export default function WorkflowPreview({ repoPath, workflow, onClose }) {
  const [view, setView] = useState(workflow.jobs?.length ? 'graph' : 'code')
  const [source, setSource] = useState(null)
  const [sourceError, setSourceError] = useState(null)

  useEffect(() => {
    if (view !== 'code' || source != null) return
    api
      .getWorkflowSource(repoPath, workflow.file)
      .then((result) => setSource(result.source))
      .catch((err) => setSourceError(`Couldn't load workflow source: ${err.message}`))
  }, [view, source, repoPath, workflow.file])

  const hasGraph = (workflow.jobs?.length || 0) > 0

  return (
    <div className="run-detail">
      <button className="drawer__close" onClick={onClose} title="Close" aria-label="Close">
        <X size={14} />
      </button>
      <div className="run-detail__head">
        <div className="run-detail__head-row">
          <h2>{workflow.name}</h2>
        </div>
        <p className="run-detail__file">{workflow.file}</p>
        <div className="run-detail__toolbar">
          <span />
          <div className="view-toggle">
            {hasGraph && (
              <button
                className={`view-toggle__btn${view === 'graph' ? ' view-toggle__btn--active' : ''}`}
                onClick={() => setView('graph')}
              >
                Graph
              </button>
            )}
            <button
              className={`view-toggle__btn${view === 'code' ? ' view-toggle__btn--active' : ''}`}
              onClick={() => setView('code')}
            >
              Code
            </button>
          </div>
        </div>
      </div>
      {view === 'graph' && hasGraph ? (
        <JobGraph jobs={workflow.jobs} runtimeJobsById={NO_RUNTIME_JOBS} runStatus={null} onSelectJob={noop} />
      ) : sourceError ? (
        <p className="error">{sourceError}</p>
      ) : source == null ? (
        <p className="empty-state">Loading source…</p>
      ) : (
        <WorkflowSource source={source} jobs={workflow.jobs || []} highlightJobId={null} highlightStepName={null} />
      )}
    </div>
  )
}
