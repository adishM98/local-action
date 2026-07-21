import { useState } from 'react'
import { api } from '../api.js'

export default function WorkflowsPanel({ repoPath, setRepoPath, onRunStarted }) {
  const [workflows, setWorkflows] = useState([])
  const [error, setError] = useState(null)
  const [selections, setSelections] = useState({})

  async function scan() {
    setError(null)
    try {
      const result = await api.scan(repoPath)
      setWorkflows(result || [])
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
    const { runId } = await api.createRun({
      repoPath,
      workflowFile: workflow.file,
      event: selection.event,
      inputs: selection.inputs,
    })
    onRunStarted(runId)
  }

  return (
    <div>
      <div className="row">
        <input
          value={repoPath}
          onChange={(e) => setRepoPath(e.target.value)}
          placeholder="/path/to/repo"
        />
        <button onClick={scan}>Scan</button>
      </div>
      {error && <p className="error">{error}</p>}
      {workflows.map((wf) => {
        const selection = selections[wf.file] || { event: '', inputs: {} }
        const dispatchInputs = wf.dispatchInputs || []
        return (
          <div className="card" key={wf.file}>
            <h3>{wf.name}</h3>
            <p>{wf.file}</p>
            {wf.parseError ? (
              <p className="error">{wf.parseError}</p>
            ) : (
              <>
                <select value={selection.event} onChange={(e) => selectEvent(wf.file, e.target.value)}>
                  <option value="">Select event</option>
                  {wf.events.map((ev) => (
                    <option key={ev} value={ev}>
                      {ev}
                    </option>
                  ))}
                </select>
                {selection.event === 'workflow_dispatch' &&
                  dispatchInputs.map((input) => (
                    <div key={input.name}>
                      <label>{input.name}</label>
                      <input
                        placeholder={input.default}
                        onChange={(e) => setInput(wf.file, input.name, e.target.value)}
                      />
                    </div>
                  ))}
                <button disabled={!selection.event} onClick={() => run(wf)}>
                  Run
                </button>
              </>
            )}
          </div>
        )
      })}
    </div>
  )
}
