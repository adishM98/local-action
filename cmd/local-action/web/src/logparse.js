// Groups raw act --json log lines into GitHub-Actions-shaped jobs/steps.
// Lines that aren't JSON (old runs, act's bare stderr) fall into `other`.
export function parseLogLines(lines) {
  const jobs = []
  const jobsById = new Map()
  const other = []

  lines.forEach((text, i) => {
    const no = i + 1
    let entry
    try {
      entry = JSON.parse(text)
    } catch {
      other.push({ no, text })
      return
    }
    if (!entry || typeof entry !== 'object' || typeof entry.msg !== 'string') {
      other.push({ no, text })
      return
    }
    const msg = entry.msg.replace(/\n$/, '')
    if (!entry.jobID) {
      other.push({ no, text: msg })
      return
    }

    let job = jobsById.get(entry.jobID)
    if (!job) {
      job = {
        id: entry.jobID,
        name: entry.job || entry.jobID,
        result: null,
        steps: [],
        stepsByName: new Map(),
        tail: [],
        durationMs: null,
        firstTimeMs: null,
      }
      jobsById.set(entry.jobID, job)
      jobs.push(job)
    }
    if (entry.jobResult) job.result = entry.jobResult

    const entryTimeMs = entry.time ? Date.parse(entry.time) : NaN
    if (!Number.isNaN(entryTimeMs)) {
      if (job.firstTimeMs == null) job.firstTimeMs = entryTimeMs
      job.durationMs = entryTimeMs - job.firstTimeMs
    }

    if (!entry.step) {
      job.tail.push({ no, text: msg })
      return
    }
    let step = job.stepsByName.get(entry.step)
    if (!step) {
      step = { name: entry.step, result: null, lines: [], durationMs: null }
      job.stepsByName.set(entry.step, step)
      job.steps.push(step)
    }
    step.lines.push({ no, text: msg })
    if (entry.stepResult) step.result = entry.stepResult
    if (typeof entry.executionTime === 'number') {
      step.durationMs = Math.round(entry.executionTime / 1e6)
    }
  })

  return { jobs: jobs.map(({ stepsByName, firstTimeMs, ...job }) => job), other }
}
