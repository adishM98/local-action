import { useEffect, useMemo, useState } from 'react'
import { X, ChevronRight } from 'lucide-react'
import { api } from '../api.js'
import StatusIcon, { StatusBadge } from './StatusIcon.jsx'
import JobGraph from './JobGraph.jsx'
import WorkflowSource from './WorkflowSource.jsx'
import { relativeTime, duration, formatDurationMs } from '../format.js'
import { parseLogLines, liveStatus } from '../logparse.js'

const TERMINAL = ['success', 'failed', 'cancelled']

// While the run is live: WS streams lines, a 2s poll tracks status. On any
// terminal poll the persisted log replaces the streamed lines wholesale, so
// WS hiccups can't lose output — SQLite is the source of truth at the end.
export default function RunDetail({ runId, workflows, onClose, onOpenRun }) {
  const [run, setRun] = useState(null)
  const [lines, setLines] = useState([])
  const [error, setError] = useState(null)
  const [wsDown, setWsDown] = useState(false)
  const [busy, setBusy] = useState(false)
  const [view, setView] = useState('graph')
  const [source, setSource] = useState(null)
  const [sourceError, setSourceError] = useState(null)
  const [fileChanged, setFileChanged] = useState(false)

  useEffect(() => {
    setRun(null)
    setLines([])
    setError(null)
    setWsDown(false)
    setView('graph')
    setSource(null)
    setSourceError(null)
    setFileChanged(false)
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
  const workflow = run && workflows?.find((w) => w.file === run.workflowFile)
  const workflowName = workflow?.name || run?.workflowFile
  const workflowJobs = workflow?.jobs || []
  const hasGraph = workflowJobs.length > 0
  const runtimeJobsById = useMemo(() => new Map(parsed.jobs.map((j) => [j.id, j])), [parsed.jobs])
  const failedJob = parsed.jobs.find((j) => j.result === 'failure')
  const failedJobId = failedJob?.id || null
  // act doesn't always mark the step that actually broke the job as
  // "failure" (see the Docker-build-and-push case that prompted this) — the
  // step still mid-flight when the job died is the next best signal.
  const failedStepName = failedJob
    ? failedJob.steps.find((s) => s.result === 'failure')?.name || failedJob.steps[failedJob.steps.length - 1]?.name || null
    : null
  const effectiveView = !hasGraph && view === 'graph' ? 'logs' : view

  useEffect(() => {
    if (view !== 'code' || source != null || !run) return
    api
      .getWorkflowSource(run.repoPath, run.workflowFile)
      .then((result) => setSource(result.source))
      .catch((err) => setSourceError(`Couldn't load workflow source: ${err.message}`))
  }, [view, source, run])

  // Detects an edit made outside the app (in an editor, a git checkout,
  // etc.) while this run's detail view is open — mtime is enough, no need
  // to read/hash the file just to answer "has anything happened to this
  // since the run started." Keeps polling even after it fires since the
  // file could be edited again; keeps polling regardless of run status
  // since re-running a still-in-progress run is a legitimate (if unusual)
  // choice, same as GitHub's own "re-run" always being available.
  useEffect(() => {
    if (!run) return
    let cancelled = false
    function check() {
      api
        .getWorkflowFileMTime(run.repoPath, run.workflowFile)
        .then((result) => {
          if (!cancelled && result.mtime > run.createdAt) setFileChanged(true)
        })
        .catch(() => {})
    }
    check()
    const interval = setInterval(check, 3000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [run?.repoPath, run?.workflowFile, run?.createdAt])

  function selectJob(jobId) {
    setView('logs')
    requestAnimationFrame(() => {
      document.getElementById(`job-${jobId}`)?.scrollIntoView({ block: 'nearest' })
    })
  }

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
      <button className="drawer__close" onClick={onClose} title="Close" aria-label="Close">
        <X size={14} />
      </button>
      <div className="run-detail__head">
        <div className="run-detail__head-row">
          <StatusBadge status={run?.status} />
          <h2 title={run ? `${run.workflowFile} #${run.id}` : `Run #${runId}`}>
            {run ? `${workflowName} #${run.id}` : `Run #${runId}`}
          </h2>
        </div>
        {run && <p className="run-detail__file">{run.workflowFile}</p>}
        <div className="run-detail__toolbar">
          {run && (
            <span className="run-detail__meta">
              {run.event}
              {run.branch && ` · ${run.branch}`}
              {run.commitSha && ` (${run.commitSha})`} · {relativeTime(run.createdAt)}
              {duration(run) && ` · ${duration(run)}`}
            </span>
          )}
          <div className="run-detail__actions">
            <div className="view-toggle">
              {hasGraph && (
                <button
                  className={`view-toggle__btn${effectiveView === 'graph' ? ' view-toggle__btn--active' : ''}`}
                  onClick={() => setView('graph')}
                >
                  Graph
                </button>
              )}
              <button
                className={`view-toggle__btn${effectiveView === 'logs' ? ' view-toggle__btn--active' : ''}`}
                onClick={() => setView('logs')}
              >
                Logs
              </button>
              <button
                className={`view-toggle__btn${effectiveView === 'code' ? ' view-toggle__btn--active' : ''}`}
                onClick={() => setView('code')}
              >
                Code
              </button>
            </div>
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
      </div>
      {wsDown && run && !isTerminal && (
        <div className="banner banner--warn">
          Live stream lost — falling back to polling. Output may lag a couple of seconds.
        </div>
      )}
      {fileChanged && run && (
        <div className="banner banner--warn">
          <span>{run.workflowFile} has changed on disk since this run.</span>
          <button className="btn" onClick={rerun} disabled={busy}>
            Re-run with same inputs
          </button>
        </div>
      )}
      {error && <p className="error">{error}</p>}
      {effectiveView === 'graph' ? (
        <JobGraph
          jobs={workflowJobs}
          runtimeJobsById={runtimeJobsById}
          runStatus={run?.status}
          onSelectJob={selectJob}
        />
      ) : effectiveView === 'code' ? (
        sourceError ? (
          <p className="error">{sourceError}</p>
        ) : source == null ? (
          <p className="empty-state">Loading source…</p>
        ) : (
          <WorkflowSource
            source={source}
            jobs={workflowJobs}
            highlightJobId={failedJobId}
            highlightStepName={failedStepName}
          />
        )
      ) : (
        <>
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
        </>
      )}
    </div>
  )
}

function JobCard({ job, runStatus }) {
  const running = job.result == null && runStatus === 'running'
  const completedSteps = job.steps.filter((s) => s.result).length

  return (
    <section className="job-card" id={`job-${job.id}`}>
      <header className="job-card__head">
        <StatusIcon status={liveStatus(job.result, runStatus)} />
        <h3>{job.name}</h3>
        {running && job.steps.length > 0 && (
          <span className="job-card__progress" title="Steps observed so far — the workflow's true total isn't known until it finishes">
            Step {completedSteps}/{job.steps.length}
          </span>
        )}
        {job.durationMs != null && <span className="job-card__duration">{formatDurationMs(job.durationMs)}</span>}
      </header>
      {running && job.steps.length > 0 && (
        <div className="job-card__progress-bar">
          <div className="job-card__progress-fill" style={{ width: `${(completedSteps / job.steps.length) * 100}%` }} />
        </div>
      )}
      {job.steps.map((step) => (
        <StepRow key={step.name} step={step} runStatus={runStatus} />
      ))}
      {job.tail.length > 0 && (
        <div className="job-card__tail">
          {job.tail.map((l) => (
            <div className="log-line" key={l.no}>
              <span className="log-line__no">{l.no}</span>
              <span className="log-line__text">{l.text}</span>
            </div>
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
        <ChevronRight size={13} className={`step__chevron${open ? ' step__chevron--open' : ''}`} />
        <StatusIcon status={liveStatus(step.result, runStatus)} />
        <span className="step__name">{step.name}</span>
        {step.durationMs != null && <span className="step__duration">{formatDurationMs(step.durationMs)}</span>}
      </button>
      {open && (
        <div className="step__lines">
          {step.lines.map((l) => (
            <div className="log-line" key={l.no}>
              <span className="log-line__no">{l.no}</span>
              <span className="log-line__text">{l.text}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
