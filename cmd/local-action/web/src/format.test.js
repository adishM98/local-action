import test from 'node:test'
import assert from 'node:assert/strict'
import { formatDurationMs } from './format.js'

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
