import { useState } from 'react'
import { api } from '../api.js'
import { rememberPath } from './PromptBar.jsx'

export default function WorkflowsPanel({ repoPath, onRunStarted }) {
  const [workflows, setWorkflows] = useState([])
  const [scanned, setScanned] = useState(false)
  const [error, setError] = useState(null)
  const [selections, setSelections] = useState({})
  const [copiedFile, setCopiedFile] = useState(null)
  const [runError, setRunError] = useState(null)

  async function scan() {
    setError(null)
    try {
      const result = await api.scan(repoPath)
      setWorkflows(result || [])
      setScanned(true)
      rememberPath(repoPath)
    } catch (err) {
      setError(err.message)
    }
  }

  function selectEvent(file, event) {
    setSelections((prev) => ({ ...prev, [file]: { event, inputs: {} } }))
  }

  function setInput(file, name, value) {
    setSelections((prev) => ({
      ...prev,
      [file]: { ...prev[file], inputs: { ...prev[file].inputs, [name]: value } },
    }))
  }

  async function run(workflow) {
    const selection = selections[workflow.file]
    if (!selection) return
    setRunError(null)
    try {
      const { runId } = await api.createRun({
        repoPath,
        workflowFile: workflow.file,
        event: selection.event,
        inputs: selection.inputs,
      })
      onRunStarted(runId)
    } catch (err) {
      setRunError({ file: workflow.file, message: err.message })
    }
  }

  async function copyFile(file) {
    try {
      await navigator.clipboard.writeText(file)
      setCopiedFile(file)
      setTimeout(() => setCopiedFile((current) => (current === file ? null : current)), 1500)
    } catch (err) {
      console.error('copy workflow file path failed:', err)
    }
  }

  return (
    <div>
      <div className="row">
        <button className="primary" onClick={scan} disabled={!repoPath}>
          Scan
        </button>
      </div>
      {error && <p className="error">{error}</p>}
      {scanned && workflows.length === 0 && !error && (
        <p className="empty-state">No workflows found under .github/workflows in this repo.</p>
      )}
      {!scanned && !error && (
        <p className="empty-state">Enter a repo path above and scan to list its workflows.</p>
      )}
      {workflows.map((wf) => {
        const selection = selections[wf.file] || { event: '', inputs: {} }
        const dispatchInputs = wf.dispatchInputs || []
        return (
          <div className="card" key={wf.file}>
            <h3>{wf.name}</h3>
            <div className="card__path">
              {wf.file}
              <button
                className={`icon-btn${copiedFile === wf.file ? ' copied' : ''}`}
                onClick={() => copyFile(wf.file)}
                title="Copy workflow file path"
                aria-label="Copy workflow file path"
              >
                {copiedFile === wf.file ? '✓' : '⧉'}
              </button>
            </div>
            {wf.parseError ? (
              <p className="error">{wf.parseError}</p>
            ) : (
              <>
                <div className="row">
                  <select value={selection.event} onChange={(e) => selectEvent(wf.file, e.target.value)}>
                    <option value="">Select event</option>
                    {wf.events.map((ev) => (
                      <option key={ev} value={ev}>
                        {ev}
                      </option>
                    ))}
                  </select>
                  <button className="primary" disabled={!selection.event} onClick={() => run(wf)}>
                    Run
                  </button>
                </div>
                {runError?.file === wf.file && <p className="error">{runError.message}</p>}
                {selection.event === 'workflow_dispatch' &&
                  dispatchInputs.map((input) => (
                    <div className="row" key={input.name}>
                      <label>{input.name}</label>
                      <input
                        placeholder={input.default}
                        onChange={(e) => setInput(wf.file, input.name, e.target.value)}
                      />
                    </div>
                  ))}
              </>
            )}
          </div>
        )
      })}
    </div>
  )
}
