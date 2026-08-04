// Topological-level layout for a workflow's job dependency graph. Jobs are
// grouped into columns by longest-path-from-root depth, so a fan-in job
// (needs: [a, b]) always renders to the right of both a and b even when
// they're at different depths. Cycles and dangling `needs` references (a
// name that doesn't match any job id) are ignored rather than thrown on —
// scanner-side YAML can be malformed and the graph should still render
// something rather than crash the run detail view.
export function computeGraphLevels(jobs) {
  const byId = new Map(jobs.map((j) => [j.id, j]))
  const depth = new Map()

  function depthOf(id, seen) {
    if (depth.has(id)) return depth.get(id)
    if (seen.has(id)) return 0 // cycle guard
    seen.add(id)
    const job = byId.get(id)
    const needs = (job?.needs || []).filter((n) => byId.has(n))
    const d = needs.length === 0 ? 0 : 1 + Math.max(...needs.map((n) => depthOf(n, seen)))
    depth.set(id, d)
    return d
  }

  jobs.forEach((j) => depthOf(j.id, new Set()))

  const levels = []
  jobs.forEach((j) => {
    const d = depth.get(j.id)
    if (!levels[d]) levels[d] = []
    levels[d].push(j.id)
  })
  return levels.filter(Boolean)
}
