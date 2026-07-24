import { useEffect, useRef, useState } from 'react'
import { api } from '../api.js'

export default function RunWorkflowMenu({ repoPath, workflow, onStarted, onOpenSecrets }) {
  const [open, setOpen] = useState(false)
  const [event, setEvent] = useState(workflow.events?.[0] || '')
  const [inputs, setInputs] = useState({})
  const [counts, setCounts] = useState(null)
  const [error, setError] = useState(null)
  const [starting, setStarting] = useState(false)
  const [payloadOpen, setPayloadOpen] = useState(false)
  const [payload, setPayload] = useState('')
  const [payloadError, setPayloadError] = useState(null)
  const [autoDetected, setAutoDetected] = useState(false)
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

  // No saved payload yet? Fall back to the workflow's auto-detected one
  // (derived server-side from a solvable if: condition) so a labeled/PR-
  // gated job just runs, instead of requiring the user to hand-write JSON.
  useEffect(() => {
    if (!open) return
    api
      .getEventPayload(repoPath, workflow.file)
      .then((result) => {
        const saved = result?.payload || ''
        if (saved) {
          setPayload(saved)
          setAutoDetected(false)
        } else if (workflow.autoEventPayload) {
          setPayload(workflow.autoEventPayload)
          setAutoDetected(true)
        } else {
          setPayload('')
          setAutoDetected(false)
        }
      })
      .catch(() => setPayload(''))
  }, [open, repoPath, workflow.file])

  async function run() {
    const trimmed = payload.trim()
    if (trimmed) {
      try {
        JSON.parse(trimmed)
      } catch {
        setPayloadError('Not valid JSON')
        setPayloadOpen(true)
        return
      }
    }
    setPayloadError(null)
    setStarting(true)
    setError(null)
    try {
      await api.saveEventPayload(repoPath, workflow.file, trimmed)
      const { runId } = await api.createRun({
        repoPath,
        workflowFile: workflow.file,
        event,
        inputs,
        eventPayload: trimmed,
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
          <div className="disclosure">
            <button className="linklike" onClick={() => setPayloadOpen(!payloadOpen)}>
              Event payload (JSON) {payloadOpen ? '▾' : '▸'}
            </button>
            {payloadOpen && (
              <div className="field">
                <textarea
                  className="event-payload-input"
                  rows={4}
                  placeholder={'{\n  "action": "labeled",\n  "label": { "name": "run-ci" }\n}'}
                  value={payload}
                  onChange={(e) => {
                    setPayload(e.target.value)
                    setPayloadError(null)
                    setAutoDetected(false)
                  }}
                />
                <small>
                  {autoDetected
                    ? "Auto-detected from this workflow's if: condition — edit or clear for a different scenario."
                    : 'Fills github.event.* so if: conditions gated on event data can run locally.'}
                </small>
                {payloadError && <p className="error">{payloadError}</p>}
              </div>
            )}
          </div>
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
