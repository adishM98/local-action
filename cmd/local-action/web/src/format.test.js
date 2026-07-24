import test from 'node:test'
import assert from 'node:assert/strict'
import { formatDurationMs, computeRunStats } from './format.js'

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
