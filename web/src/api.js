async function request(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new Error(await res.text())
  }
  if (res.status === 204) return null
  return res.json()
}

export const api = {
  health: () => request('GET', '/api/health'),
  scan: (path) => request('POST', '/api/scan', { path }),
  repoInfo: (repoPath) => request('GET', `/api/repo-info?repoPath=${encodeURIComponent(repoPath)}`),
  listSecrets: (repoPath, kind) =>
    request('GET', `/api/secrets?repoPath=${encodeURIComponent(repoPath)}&kind=${kind}`),
  upsertSecret: (repoPath, kind, key, value, workflowFile = '') =>
    request('POST', '/api/secrets', { repoPath, kind, key, value, workflowFile }),
  deleteSecret: (repoPath, kind, key, workflowFile = '') =>
    request('DELETE', '/api/secrets', { repoPath, kind, key, workflowFile }),
  getEventPayload: (repoPath, workflowFile) =>
    request('GET', `/api/event-payload?repoPath=${encodeURIComponent(repoPath)}&workflowFile=${encodeURIComponent(workflowFile)}`),
  saveEventPayload: (repoPath, workflowFile, payload) =>
    request('POST', '/api/event-payload', { repoPath, workflowFile, payload }),
  getWorkflowCategories: (repoPath) =>
    request('GET', `/api/workflow-categories?repoPath=${encodeURIComponent(repoPath)}`),
  saveWorkflowCategory: (repoPath, workflowFile, category) =>
    request('POST', '/api/workflow-categories', { repoPath, workflowFile, category }),
  createRun: (payload) => request('POST', '/api/runs', payload),
  listRuns: (repoPath) => request('GET', `/api/runs?repoPath=${encodeURIComponent(repoPath)}`),
  getRun: (id) => request('GET', `/api/runs/${id}`),
  cancelRun: (id) => request('POST', `/api/runs/${id}/cancel`),
}
