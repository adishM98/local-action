import { useEffect, useRef, useState } from 'react'
import { api } from '../api.js'

export default function RunWorkflowMenu({ repoPath, workflow, onStarted, onOpenSecrets }) {
  const [open, setOpen] = useState(false)
  const [event, setEvent] = useState(workflow.events?.[0] || '')
  const [inputs, setInputs] = useState({})
  const [counts, setCounts] = useState(null)
  const [error, setError] = useState(null)
  const [starting, setStarting] = useState(false)
  const ref = useRef(null)

  useEffect(() => {
    if (!open) return
    function onDocClick(e) {
      if (ref.current && !ref.current.contains(e.target)) setOpen(false)
    }
    document.addEventListener('mousedown', onDocClick)
    return () => document.removeEventListener('mousedown', onDocClick)
  }, [open])

  useEffect(() => {
    if (!open) return
    Promise.all([api.listSecrets(repoPath, 'secret'), api.listSecrets(repoPath, 'var')])
      .then(([secrets, vars]) => {
        const relevant = (list) =>
          (list || []).filter((e) => !e.workflowFile || e.workflowFile === workflow.file).length
        setCounts({ secrets: relevant(secrets), vars: relevant(vars) })
      })
      .catch(() => setCounts(null))
  }, [open, repoPath, workflow.file])

  async function run() {
    setStarting(true)
    setError(null)
    try {
      const { runId } = await api.createRun({
        repoPath,
        workflowFile: workflow.file,
        event,
        inputs,
      })
      setOpen(false)
      onStarted(runId)
    } catch (err) {
      setError(err.message)
    } finally {
      setStarting(false)
    }
  }

  const dispatchInputs = event === 'workflow_dispatch' ? workflow.dispatchInputs || [] : []

  return (
    <div className="run-menu" ref={ref}>
      <button className="btn btn--primary" onClick={() => setOpen(!open)}>
        Run workflow ▾
      </button>
      {open && (
        <div className="run-menu__panel">
          <label className="field">
            <span>Event</span>
            <select value={event} onChange={(e) => setEvent(e.target.value)}>
              {(workflow.events || []).map((ev) => (
                <option key={ev} value={ev}>
                  {ev}
                </option>
              ))}
            </select>
          </label>
          {dispatchInputs.map((input) => (
            <label className="field" key={input.name}>
              <span>
                {input.name}
                {input.required ? ' *' : ''}
              </span>
              {input.type === 'choice' && input.options?.length ? (
                <select
                  value={inputs[input.name] ?? input.default ?? ''}
                  onChange={(e) => setInputs({ ...inputs, [input.name]: e.target.value })}
                >
                  {input.options.map((o) => (
                    <option key={o} value={o}>
                      {o}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  placeholder={input.default}
                  value={inputs[input.name] || ''}
                  onChange={(e) => setInputs({ ...inputs, [input.name]: e.target.value })}
                />
              )}
              {input.description && <small>{input.description}</small>}
            </label>
          ))}
          {counts && (
            <p>
              <button className="linklike" onClick={() => onOpenSecrets(workflow.file)}>
                {counts.secrets} secret{counts.secrets === 1 ? '' : 's'}, {counts.vars} var
                {counts.vars === 1 ? '' : 's'} will be injected
              </button>
            </p>
          )}
          {error && <p className="error">{error}</p>}
          <button className="btn btn--primary btn--block" onClick={run} disabled={!event || starting}>
            {starting ? 'Starting…' : 'Run workflow'}
          </button>
        </div>
      )}
    </div>
  )
}
