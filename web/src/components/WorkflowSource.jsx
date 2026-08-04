import { useEffect, useRef } from 'react'

// Jobs are already in file order (see JobInfo.Line in scanner.go) — each
// job's block runs from its own line to just before the next job's line,
// or EOF for the last one.
function jobRange(job, jobs, totalLines) {
  const sorted = jobs.filter((j) => j.line).sort((a, b) => a.line - b.line)
  const i = sorted.findIndex((j) => j.id === job.id)
  if (i === -1) return null
  return { start: job.line, end: i + 1 < sorted.length ? sorted[i + 1].line - 1 : totalLines }
}

// Same idea one level down: a step's block runs to just before the next
// step in the SAME job (falling back to the job's own end for the last
// step). Steps without a declared name: are included for this boundary
// math but can never be the highlight target — see StepInfo.Name in
// scanner.go for why act's synthesized label can't be matched back to one.
function stepRange(stepName, job, jobBounds) {
  const steps = job.steps || []
  const i = steps.findIndex((s) => s.name === stepName)
  if (i === -1) return null
  return {
    start: steps[i].line,
    end: i + 1 < steps.length ? steps[i + 1].line - 1 : jobBounds.end,
  }
}

export default function WorkflowSource({ source, jobs, highlightJobId, highlightStepName }) {
  const containerRef = useRef(null)
  const lines = source.split('\n')
  const job = jobs.find((j) => j.id === highlightJobId)
  const jobBounds = job ? jobRange(job, jobs, lines.length) : null
  const highlight = (job && jobBounds && stepRange(highlightStepName, job, jobBounds)) || jobBounds

  useEffect(() => {
    if (!highlight) return
    containerRef.current?.querySelector(`[data-line="${highlight.start}"]`)?.scrollIntoView({ block: 'center' })
  }, [highlightJobId, highlightStepName])

  return (
    <div className="workflow-source" ref={containerRef}>
      {lines.map((text, i) => {
        const no = i + 1
        const failed = highlight && no >= highlight.start && no <= highlight.end
        return (
          <div key={no} data-line={no} className={`log-line workflow-source__line${failed ? ' workflow-source__line--failed' : ''}`}>
            <span className="log-line__no">{no}</span>
            <span className="log-line__text">{text}</span>
          </div>
        )
      })}
    </div>
  )
}
