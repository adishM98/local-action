import test from 'node:test'
import assert from 'node:assert/strict'
import { computeGraphLevels } from './graph.js'

test('chain: each job needs the previous one', () => {
  const jobs = [
    { id: 'build', needs: [] },
    { id: 'test', needs: ['build'] },
    { id: 'deploy', needs: ['test'] },
  ]
  assert.deepEqual(computeGraphLevels(jobs), [['build'], ['test'], ['deploy']])
})

test('fan-in: a job needing two different-depth jobs lands past both', () => {
  const jobs = [
    { id: 'build', needs: [] },
    { id: 'test', needs: ['build'] },
    { id: 'deploy', needs: ['build', 'test'] },
  ]
  assert.deepEqual(computeGraphLevels(jobs), [['build'], ['test'], ['deploy']])
})

test('fan-out: independent jobs needing the same root share a column', () => {
  const jobs = [
    { id: 'build', needs: [] },
    { id: 'lint', needs: ['build'] },
    { id: 'test', needs: ['build'] },
  ]
  assert.deepEqual(computeGraphLevels(jobs), [['build'], ['lint', 'test']])
})

test('independent jobs with no needs all land in column 0', () => {
  const jobs = [{ id: 'a', needs: [] }, { id: 'b', needs: [] }]
  assert.deepEqual(computeGraphLevels(jobs), [['a', 'b']])
})

test('dangling needs reference is ignored, not thrown', () => {
  const jobs = [{ id: 'a', needs: ['does-not-exist'] }]
  assert.deepEqual(computeGraphLevels(jobs), [['a']])
})

test('cycle is broken rather than infinite-looping', () => {
  const jobs = [
    { id: 'a', needs: ['b'] },
    { id: 'b', needs: ['a'] },
  ]
  assert.doesNotThrow(() => computeGraphLevels(jobs))
})
