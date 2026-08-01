import test from 'node:test'
import assert from 'node:assert/strict'
import {
  formatDurationMs,
  computeRunStats,
  longestRunningWorkflow,
  dailyTrend,
  lastRunByWorkflow,
  lastStatusByWorkflow,
  filterRuns,
} from './format.js'

const run = (status, startedAt, finishedAt, createdAt) => ({
  status,
  createdAt: createdAt ?? null,
  startedAt: startedAt == null ? { Valid: false } : { Valid: true, Int64: startedAt },
  finishedAt: finishedAt == null ? { Valid: false } : { Valid: true, Int64: finishedAt },
})

test('computeRunStats: empty runs list', () => {
  const stats = computeRunStats([])
  assert.deepEqual(stats, {
    total: 0,
    passed: 0,
    failed: 0,
    running: 0,
    cancelled: 0,
    avgDurationMs: null,
    avgQueueMs: null,
  })
})

test('computeRunStats: avgQueueMs averages createdAt-to-startedAt wait', () => {
  const runs = [run('success', 110, 120, 100), run('success', 130, 140, 100)]
  // waits: 10s and 30s -> avg 20s = 20000ms
  assert.equal(computeRunStats(runs).avgQueueMs, 20000)
})

test('computeRunStats: avgQueueMs excludes runs that never started', () => {
  const runs = [run('success', 110, 120, 100), run('queued', null, null, 100)]
  assert.equal(computeRunStats(runs).avgQueueMs, 10000)
})

test('computeRunStats: avgQueueMs is null when nothing has started', () => {
  assert.equal(computeRunStats([run('queued', null, null, 100)]).avgQueueMs, null)
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
  assert.equal(stats.cancelled, 1)
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

test('filterRuns: branch filter', () => {
  const runs = [
    { id: 1, workflowFile: 'ci.yml', event: 'push', status: 'success', branch: 'main' },
    { id: 2, workflowFile: 'ci.yml', event: 'push', status: 'success', branch: 'feature/x' },
  ]
  assert.deepEqual(filterRuns(runs, { search: '', status: '', event: '', branch: 'main' }, nameFor), [runs[0]])
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

test('longestRunningWorkflow: picks the longest finished run', () => {
  const runs = [
    { ...run('success', 100, 110), workflowFile: 'ci.yml' }, // 10s
    { ...run('success', 100, 300), workflowFile: 'deploy.yml' }, // 200s, longest
    { ...run('running', 100, null), workflowFile: 'never-finishes.yml' },
  ]
  const got = longestRunningWorkflow(runs, (r) => r.workflowFile)
  assert.equal(got.name, 'deploy.yml')
  assert.equal(got.durationMs, 200000)
})

test('longestRunningWorkflow: null when nothing has finished', () => {
  assert.equal(longestRunningWorkflow([run('running', 100, null)], (r) => r.workflowFile), null)
})

test('dailyTrend: buckets terminal runs by day and computes success rate', () => {
  // Match dailyTrend's own bucketing (local calendar day, hours zeroed) so
  // the test is timezone-independent — noon avoids any DST-shift edge case.
  const localMidnight = new Date()
  localMidnight.setHours(12, 0, 0, 0)
  const today = Math.floor(localMidnight.getTime() / 1000)
  const yesterday = today - 86400
  const runs = [
    run('success', today, today + 10, today),
    run('success', today, today + 10, today),
    run('failed', today, today + 10, today),
    run('success', yesterday, yesterday + 10, yesterday),
  ]
  const trend = dailyTrend(runs, 7)
  assert.equal(trend.length, 7)
  const todayBucket = trend[trend.length - 1]
  const yesterdayBucket = trend[trend.length - 2]
  assert.equal(todayBucket.pct, 67) // 2/3 rounded
  assert.equal(yesterdayBucket.pct, 100)
})

test('dailyTrend: a day with no terminal runs gets pct null', () => {
  const trend = dailyTrend([], 7)
  assert.ok(trend.every((b) => b.pct === null))
})

test('lastRunByWorkflow: first occurrence per file wins, returns the full run', () => {
  const runs = [
    { id: 2, workflowFile: 'ci.yml', status: 'failed', branch: 'main' },
    { id: 1, workflowFile: 'ci.yml', status: 'success', branch: 'main' },
  ]
  const byFile = lastRunByWorkflow(runs)
  assert.equal(byFile['ci.yml'].id, 2)
  assert.equal(byFile['deploy.yml'], undefined)
})

test('lastStatusByWorkflow: first occurrence per file wins (newest-first input)', () => {
  const runs = [
    { workflowFile: 'ci.yml', status: 'failed' }, // newest for ci.yml
    { workflowFile: 'deploy.yml', status: 'success' },
    { workflowFile: 'ci.yml', status: 'success' }, // older, ignored
  ]
  assert.deepEqual(lastStatusByWorkflow(runs), { 'ci.yml': 'failed', 'deploy.yml': 'success' })
})

test('lastStatusByWorkflow: empty runs list', () => {
  assert.deepEqual(lastStatusByWorkflow([]), {})
})
