import { useEffect, useState } from 'react'
import * as Popover from '@radix-ui/react-popover'
import { api } from '../api.js'

// Inverse of the payload the label dropdown builds — recovers the
// previously-picked label (or '' for "None") from a saved payload so
// reopening the menu shows the same selection instead of resetting it.
function labelFromPayload(jsonStr) {
  if (!jsonStr) return ''
  try {
    return JSON.parse(jsonStr)?.pull_request?.labels?.[0]?.name || ''
  } catch {
    return ''
  }
}

export default function RunWorkflowMenu({ repoPath, workflow, onStarted, onOpenSecrets }) {
  const [open, setOpen] = useState(false)
  const [event, setEvent] = useState(workflow.events?.[0] || '')
  const [inputs, setInputs] = useState({})
  const [counts, setCounts] = useState(null)
  const [error, setError] = useState(null)
  const [starting, setStarting] = useState(false)
  const [payload, setPayload] = useState('')
  const [payloadError, setPayloadError] = useState(null)
  const [selectedLabel, setSelectedLabel] = useState('')

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

  // Manual entry is only needed when the workflow's if: condition couldn't
  // be auto-solved server-side. When it was solved, there's nothing to
  // show or fetch — the derived payload is used directly on run.
  const autoPayload = workflow.autoEventPayload || ''
  // Manual entry is only load-bearing when the workflow's if: condition
  // actually references event data (needsEventPayload) but couldn't be
  // auto-solved. A workflow whose condition doesn't touch github.event.*
  // at all (or has no if: at all) has nothing for a payload to do —
  // showing the field there was pure noise with no effect on the run.
  const needsManualPayload = workflow.needsEventPayload && !autoPayload

  const suggested = workflow.suggestedEventPayload || ''
  const suggestedLabels = workflow.suggestedLabels || []
  // When the if: condition is purely a set of PR-label checks, offer a
  // dropdown instead of a raw JSON field — most people testing a
  // label-gated job want "run with label X" or "confirm it skips with
  // none", not to hand-write {"pull_request":{"labels":[...]}}.
  const useLabelDropdown = needsManualPayload && suggestedLabels.length > 0

  useEffect(() => {
    if (!open || !needsManualPayload) return
    // Priority: a value the user already saved for this workflow > our
    // best-effort guess from its if: condition > empty (generic example
    // shown only as a placeholder hint, not real content).
    api
      .getEventPayload(repoPath, workflow.file)
      .then((result) => {
        if (useLabelDropdown) {
          setSelectedLabel(labelFromPayload(result?.payload))
        } else {
          setPayload(result?.payload || suggested)
        }
      })
      .catch(() => {
        if (!useLabelDropdown) setPayload(suggested)
      })
  }, [open, repoPath, workflow.file, needsManualPayload, useLabelDropdown, suggested])

  async function run() {
    const manual = needsManualPayload
      ? useLabelDropdown
        ? JSON.stringify({ pull_request: { labels: selectedLabel ? [{ name: selectedLabel }] : [] } })
        : payload.trim()
      : ''
    if (needsManualPayload && !useLabelDropdown && manual) {
      try {
        JSON.parse(manual)
      } catch {
        setPayloadError('Not valid JSON')
        return
      }
    }
    setPayloadError(null)
    setStarting(true)
    setError(null)
    try {
      if (needsManualPayload) {
        await api.saveEventPayload(repoPath, workflow.file, manual)
      }
      const { runId } = await api.createRun({
        repoPath,
        workflowFile: workflow.file,
        event,
        inputs,
        eventPayload: autoPayload || manual,
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
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button className="btn btn--primary">Run workflow ▾</button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content className="run-menu__panel" align="end" sideOffset={6}>
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
              ) : input.type === 'boolean' ? (
                <input
                  type="checkbox"
                  className="field__checkbox"
                  checked={(inputs[input.name] ?? input.default) === 'true'}
                  onChange={(e) => setInputs({ ...inputs, [input.name]: e.target.checked ? 'true' : 'false' })}
                />
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
          {useLabelDropdown && (
            <label className="field">
              <span>Label to test</span>
              <select value={selectedLabel} onChange={(e) => setSelectedLabel(e.target.value)}>
                <option value="">None — no matching label</option>
                {suggestedLabels.map((label) => (
                  <option key={label} value={label}>
                    {label}
                  </option>
                ))}
              </select>
              <small>
                This workflow's <code>if:</code> condition checks for these PR labels — pick one to run as if the PR had
                just that label, or "None" to confirm the job correctly skips when nothing matches.
              </small>
            </label>
          )}
          {needsManualPayload && !useLabelDropdown && (
            <label className="field">
              <span>
                Event payload (JSON)
                {suggested && payload !== suggested && (
                  <button
                    type="button"
                    className="linklike field__reset"
                    onClick={() => {
                      setPayload(suggested)
                      setPayloadError(null)
                    }}
                  >
                    Reset to suggestion
                  </button>
                )}
              </span>
              <textarea
                className="event-payload-input"
                rows={4}
                placeholder={'{\n  "action": "labeled",\n  "label": { "name": "run-ci" }\n}'}
                value={payload}
                onChange={(e) => {
                  setPayload(e.target.value)
                  setPayloadError(null)
                }}
              />
              <small>
                {suggested
                  ? "This workflow's if: condition combines checks we can't fully auto-solve (e.g. it uses ||) — we've " +
                    'pre-filled a best-effort guess below from the parts we recognized. Check it before running.'
                  : (
                    <>
                      This workflow's <code>if:</code> condition couldn't be auto-detected from its trigger — fill in the
                      event JSON yourself so <code>github.event.*</code> is populated and the condition can evaluate
                      locally.
                    </>
                  )}
              </small>
              {payloadError && <p className="error">{payloadError}</p>}
            </label>
          )}
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
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}
