import { useEffect, useRef } from 'react'

// Jobs are already in file order (see JobInfo.Line in scanner.go) — each
// job's block runs from its own line to just before the next job's line,
// or EOF for the last one. Good enough for "point at the block that
// failed" without needing a real YAML range parser.
function jobRanges(jobs, totalLines) {
  const sorted = jobs.filter((j) => j.line).sort((a, b) => a.line - b.line)
  return sorted.map((job, i) => ({
    id: job.id,
    start: job.line,
    end: i + 1 < sorted.length ? sorted[i + 1].line - 1 : totalLines,
  }))
}

export default function WorkflowSource({ source, jobs, highlightJobId }) {
  const containerRef = useRef(null)
  const lines = source.split('\n')
  const ranges = jobRanges(jobs, lines.length)
  const highlight = ranges.find((r) => r.id === highlightJobId)

  useEffect(() => {
    if (!highlight) return
    containerRef.current?.querySelector(`[data-line="${highlight.start}"]`)?.scrollIntoView({ block: 'center' })
  }, [highlightJobId])

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
