import test from 'node:test'
import assert from 'node:assert/strict'
import { formatDurationMs, computeRunStats, filterRuns } from './format.js'

const run = (status, startedAt, finishedAt) => ({
  status,
  startedAt: startedAt == null ? { Valid: false } : { Valid: true, Int64: startedAt },
  finishedAt: finishedAt == null ? { Valid: false } : { Valid: true, Int64: finishedAt },
})

test('computeRunStats: empty runs list', () => {
  const stats = computeRunStats([])
  assert.deepEqual(stats, { total: 0, passed: 0, failed: 0, running: 0, avgDurationMs: null })
})

test('computeRunStats: counts by status', () => {
  const runs = [
    run('success', 100, 110),
    run('success', 100, 130),
    run('failed', 100, 120),
    run('running', 100, null),
    run('cancelled', 100, 105),
  ]
  const stats = computeRunStats(runs)
  assert.equal(stats.total, 5)
  assert.equal(stats.passed, 2)
  assert.equal(stats.failed, 1)
  assert.equal(stats.running, 1)
})

test('computeRunStats: avgDurationMs only over finished runs (excludes running)', () => {
  const runs = [
    run('success', 100, 110), // 10s
    run('failed', 100, 120), // 20s
    run('running', 100, null), // excluded — not finished
  ]
  const stats = computeRunStats(runs)
  assert.equal(stats.avgDurationMs, 15000) // (10+20)/2 = 15s = 15000ms
})

test('computeRunStats: avgDurationMs is null when nothing has finished', () => {
  const stats = computeRunStats([run('running', 100, null), run('queued', null, null)])
  assert.equal(stats.avgDurationMs, null)
})

const nameFor = (r) => (r.workflowFile === 'ci.yml' ? 'CI' : 'Deploy')

test('filterRuns: empty filters pass everything through', () => {
  const runs = [
    { id: 1, workflowFile: 'ci.yml', event: 'push', status: 'success' },
    { id: 2, workflowFile: 'deploy.yml', event: 'workflow_dispatch', status: 'failed' },
  ]
  assert.deepEqual(filterRuns(runs, { search: '', status: '', event: '' }, nameFor), runs)
})

test('filterRuns: search matches workflow name, event, id, or status (case-insensitive)', () => {
  const runs = [
    { id: 1, workflowFile: 'ci.yml', event: 'push', status: 'success' },
    { id: 2, workflowFile: 'deploy.yml', event: 'workflow_dispatch', status: 'failed' },
  ]
  assert.deepEqual(filterRuns(runs, { search: 'deploy', status: '', event: '' }, nameFor), [runs[1]])
  assert.deepEqual(filterRuns(runs, { search: 'PUSH', status: '', event: '' }, nameFor), [runs[0]])
  assert.deepEqual(filterRuns(runs, { search: '#2', status: '', event: '' }, nameFor), [runs[1]])
  assert.deepEqual(filterRuns(runs, { search: 'failed', status: '', event: '' }, nameFor), [runs[1]])
})

test('filterRuns: status and event filters combine with search (AND)', () => {
  const runs = [
    { id: 1, workflowFile: 'ci.yml', event: 'push', status: 'success' },
    { id: 2, workflowFile: 'ci.yml', event: 'push', status: 'failed' },
    { id: 3, workflowFile: 'ci.yml', event: 'workflow_dispatch', status: 'success' },
  ]
  assert.deepEqual(filterRuns(runs, { search: '', status: 'success', event: '' }, nameFor), [runs[0], runs[2]])
  assert.deepEqual(filterRuns(runs, { search: '', status: '', event: 'push' }, nameFor), [runs[0], runs[1]])
  assert.deepEqual(filterRuns(runs, { search: '', status: 'success', event: 'push' }, nameFor), [runs[0]])
})

test('formatDurationMs: null/undefined renders empty', () => {
  assert.equal(formatDurationMs(null), '')
  assert.equal(formatDurationMs(undefined), '')
})

test('formatDurationMs: sub-minute renders as seconds', () => {
  assert.equal(formatDurationMs(0), '0s')
  assert.equal(formatDurationMs(1500), '2s') // rounds
  assert.equal(formatDurationMs(59000), '59s')
})

test('formatDurationMs: minute-plus renders as Xm Ys', () => {
  assert.equal(formatDurationMs(65000), '1m 5s')
  assert.equal(formatDurationMs(732000), '12m 12s')
})
