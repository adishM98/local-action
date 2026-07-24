import test from 'node:test'
import assert from 'node:assert/strict'
import { parseLogLines } from './logparse.js'

const line = (obj) => JSON.stringify(obj)

test('groups lines into jobs and steps with results', () => {
  const lines = [
    line({ msg: 'Using docker host ...', level: 'info' }),
    line({ msg: '⭐ Run Set up job', job: 'Hello/greet', jobID: 'greet', step: 'Set up job' }),
    line({ msg: '🚀 Start image=x', job: 'Hello/greet', jobID: 'greet', step: 'Set up job' }),
    line({ msg: '✅ Success - Set up job', jobID: 'greet', job: 'Hello/greet', step: 'Set up job', stepResult: 'success' }),
    line({ msg: '⭐ Run Main echo hi', jobID: 'greet', job: 'Hello/greet', step: 'echo hi' }),
    line({ msg: 'hi\n', jobID: 'greet', job: 'Hello/greet', step: 'echo hi', raw_output: true }),
    line({ msg: '✅ Success - Main echo hi', jobID: 'greet', job: 'Hello/greet', step: 'echo hi', stepResult: 'success' }),
    line({ msg: '🏁 Job succeeded', jobID: 'greet', job: 'Hello/greet', jobResult: 'success' }),
  ]
  const { jobs, other } = parseLogLines(lines)

  assert.equal(other.length, 1) // the docker-host line has no jobID
  assert.equal(other[0].no, 1)

  assert.equal(jobs.length, 1)
  const job = jobs[0]
  assert.equal(job.id, 'greet')
  assert.equal(job.name, 'Hello/greet')
  assert.equal(job.result, 'success')
  assert.equal(job.tail.length, 1) // 🏁 line: has jobID, no step

  assert.deepEqual(job.steps.map((s) => s.name), ['Set up job', 'echo hi'])
  assert.equal(job.steps[0].result, 'success')
  assert.equal(job.steps[1].lines.some((l) => l.text === 'hi'), true) // trailing \n stripped
})

test('step duration comes from executionTime (nanoseconds -> ms)', () => {
  const lines = [
    line({ msg: '⭐ Run Main echo hi', jobID: 'g', job: 'CI/g', step: 'echo hi', time: '2026-07-24T12:00:00Z' }),
    line({
      msg: '✅ Success - Main echo hi',
      jobID: 'g',
      job: 'CI/g',
      step: 'echo hi',
      stepResult: 'success',
      executionTime: 60354791, // 60.354791ms
      time: '2026-07-24T12:00:00.060Z',
    }),
  ]
  const { jobs } = parseLogLines(lines)
  assert.equal(jobs[0].steps[0].durationMs, 60)
})

test('step with no executionTime has null durationMs', () => {
  const lines = [
    line({ msg: '⭐ Run Main build', jobID: 'g', job: 'CI/g', step: 'build' }),
  ]
  const { jobs } = parseLogLines(lines)
  assert.equal(jobs[0].steps[0].durationMs, null)
})

test('job duration spans first to last line for that job', () => {
  const lines = [
    line({ msg: 'start', jobID: 'g', job: 'CI/g', step: 'a', time: '2026-07-24T12:00:00Z' }),
    line({ msg: 'end', jobID: 'g', job: 'CI/g', jobResult: 'success', time: '2026-07-24T12:00:05Z' }),
  ]
  const { jobs } = parseLogLines(lines)
  assert.equal(jobs[0].durationMs, 5000)
})

test('job with no time fields has null durationMs', () => {
  const lines = [line({ msg: '⭐ Run Main build', jobID: 'g', job: 'CI/g', step: 'build' })]
  const { jobs } = parseLogLines(lines)
  assert.equal(jobs[0].durationMs, null)
})

test('non-JSON lines land in other, order preserved', () => {
  const { jobs, other } = parseLogLines(['plain text', 'Error: something broke'])
  assert.equal(jobs.length, 0)
  assert.deepEqual(other, [
    { no: 1, text: 'plain text' },
    { no: 2, text: 'Error: something broke' },
  ])
})

test('unresolved step has null result', () => {
  const { jobs } = parseLogLines([
    line({ msg: '⭐ Run Main build', jobID: 'b', job: 'CI/b', step: 'build' }),
  ])
  assert.equal(jobs[0].steps[0].result, null)
  assert.equal(jobs[0].result, null)
})

test('JSON line without msg string is treated as raw text', () => {
  const { jobs, other } = parseLogLines(['{"foo": 1}', '42'])
  assert.equal(jobs.length, 0)
  assert.equal(other.length, 2)
})
