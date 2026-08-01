// Pinned workflows persist per-repo in localStorage (same pattern as the
// repo header's recent-paths list) — no backend needed, this is purely a
// per-machine UI convenience.
function key(repoPath) {
  return `pinnedWorkflows:${repoPath}`
}

export function getPinned(repoPath) {
  if (!repoPath) return []
  try {
    return JSON.parse(localStorage.getItem(key(repoPath)) || '[]')
  } catch {
    return []
  }
}

export function isPinned(repoPath, workflowFile) {
  return getPinned(repoPath).includes(workflowFile)
}

export function togglePinned(repoPath, workflowFile) {
  if (!repoPath) return []
  const current = getPinned(repoPath)
  const next = current.includes(workflowFile)
    ? current.filter((f) => f !== workflowFile)
    : [...current, workflowFile]
  localStorage.setItem(key(repoPath), JSON.stringify(next))
  return next
}
