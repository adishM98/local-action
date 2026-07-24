import { useEffect, useMemo, useState } from 'react'
import { api } from '../api.js'
import StatusIcon from './StatusIcon.jsx'
import { relativeTime, duration } from '../format.js'
import { parseLogLines } from '../logparse.js'

const TERMINAL = ['success', 'failed', 'cancelled']

// While the run is live: WS streams lines, a 2s poll tracks status. On any
// terminal poll the persisted log replaces the streamed lines wholesale, so
// WS hiccups can't lose output — SQLite is the source of truth at the end.
export default function RunDetail({ runId, onBack, onOpenRun }) {
  const [run, setRun] = useState(null)
  const [lines, setLines] = useState([])
  const [error, setError] = useState(null)
  const [wsDown, setWsDown] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    setRun(null)
    setLines([])
    setError(null)
    setWsDown(false)
    let cancelled = false
    let socket = null
    let interval = null
    let wsFailed = false

    function stopLive() {
      if (interval) clearInterval(interval)
      if (socket) socket.close()
      interval = null
      socket = null
    }

    async function poll() {
      try {
        const result = await api.getRun(runId)
        if (cancelled) return
        setRun(result.run)
        if (TERMINAL.includes(result.run.status)) {
          setLines(result.logs || [])
          stopLive()
        } else if (wsFailed) {
          setLines(result.logs || [])
        }
      } catch (err) {
        if (!cancelled) setError(`Couldn't load this run: ${err.message}`)
      }
    }

    api
      .getRun(runId)
      .then((result) => {
        if (cancelled) return
        setRun(result.run)
        if (TERMINAL.includes(result.run.status)) {
          setLines(result.logs || [])
          return
        }
        const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
        socket = new WebSocket(`${protocol}://${window.location.host}/ws/runs/${runId}`)
        socket.onmessage = (event) => setLines((prev) => [...prev, event.data])
        socket.onerror = () => {
          wsFailed = true
          if (!cancelled) setWsDown(true)
        }
        interval = setInterval(poll, 2000)
      })
      .catch((err) => {
        if (!cancelled) setError(`Couldn't load this run: ${err.message}`)
      })

    return () => {
      cancelled = true
      stopLive()
    }
  }, [runId])

  const parsed = useMemo(() => parseLogLines(lines), [lines])
  const isTerminal = run && TERMINAL.includes(run.status)

  async function cancel() {
    setBusy(true)
    setError(null)
    try {
      await api.cancelRun(runId)
    } catch (err) {
      setError(`Couldn't cancel: ${err.message}`)
    } finally {
      setBusy(false)
    }
  }

  async function rerun() {
    setBusy(true)
    setError(null)
    try {
      let inputs = {}
      try {
        inputs = JSON.parse(run.inputs || '{}')
      } catch {
        inputs = {}
      }
      const { runId: newId } = await api.createRun({
        repoPath: run.repoPath,
        workflowFile: run.workflowFile,
        event: run.event,
        inputs,
      })
      onOpenRun(newId)
    } catch (err) {
      setError(`Couldn't re-run: ${err.message}`)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="run-detail">
      <button className="linklike" onClick={onBack}>
        ← All runs
      </button>
      <div className="run-detail__head">
        <StatusIcon status={run?.status} />
        <h2>{run ? `${run.workflowFile} #${run.id}` : `Run #${runId}`}</h2>
        {run && (
          <span className="run-detail__meta">
            {run.event} · {relativeTime(run.createdAt)}
            {duration(run) && ` · ${duration(run)}`}
          </span>
        )}
        <div className="run-detail__actions">
          {run && !isTerminal && (
            <button className="btn" onClick={cancel} disabled={busy}>
              Cancel
            </button>
          )}
          {isTerminal && (
            <button className="btn" onClick={rerun} disabled={busy}>
              Re-run
            </button>
          )}
        </div>
      </div>
      {wsDown && run && !isTerminal && (
        <div className="banner banner--warn">
          Live stream lost — falling back to polling. Output may lag a couple of seconds.
        </div>
      )}
      {error && <p className="error">{error}</p>}
      {parsed.jobs.map((job) => (
        <JobCard key={job.id} job={job} runStatus={run?.status} />
      ))}
      {parsed.other.length > 0 && (
        <JobCard
          job={{
            id: '_other',
            name: 'Output',
            result: null,
            steps: [{ name: 'Raw output', result: null, lines: parsed.other }],
            tail: [],
          }}
          runStatus={run?.status}
        />
      )}
      {lines.length === 0 && !error && <p className="empty-state">Waiting for output…</p>}
    </div>
  )
}

// null result while the run is live means "in progress"; after the run ends
// an unresolved step just never ran (queued glyph, muted).
function liveStatus(result, runStatus) {
  if (result) return result
  return runStatus === 'running' ? 'running' : 'queued'
}

function JobCard({ job, runStatus }) {
  return (
    <section className="job-card">
      <header className="job-card__head">
        <StatusIcon status={liveStatus(job.result, runStatus)} />
        <h3>{job.name}</h3>
      </header>
      {job.steps.map((step) => (
        <StepRow key={step.name} step={step} runStatus={runStatus} />
      ))}
      {job.tail.length > 0 && (
        <div className="job-card__tail">
          {job.tail.map((l) => (
            <div key={l.no}>{l.text}</div>
          ))}
        </div>
      )}
    </section>
  )
}

function StepRow({ step, runStatus }) {
  // GitHub behavior: steps auto-expand while unresolved or failed,
  // auto-collapse on success. A manual toggle always wins afterwards.
  const [userOpen, setUserOpen] = useState(null)
  const autoOpen = step.result == null || step.result === 'failure'
  const open = userOpen ?? autoOpen

  return (
    <div className="step">
      <button className="step__header" onClick={() => setUserOpen(!open)}>
        <span className="step__chevron">{open ? '▾' : '▸'}</span>
        <StatusIcon status={liveStatus(step.result, runStatus)} />
        <span className="step__name">{step.name}</span>
      </button>
      {open && (
        <ol className="step__lines">
          {step.lines.map((l) => (
            <li key={l.no} value={l.no}>
              {l.text}
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
