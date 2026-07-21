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
  listSecrets: (repoPath, kind) =>
    request('GET', `/api/secrets?repoPath=${encodeURIComponent(repoPath)}&kind=${kind}`),
  upsertSecret: (repoPath, kind, key, value) =>
    request('POST', '/api/secrets', { repoPath, kind, key, value }),
  deleteSecret: (repoPath, kind, key) => request('DELETE', '/api/secrets', { repoPath, kind, key }),
  createRun: (payload) => request('POST', '/api/runs', payload),
  listRuns: (repoPath) => request('GET', `/api/runs?repoPath=${encodeURIComponent(repoPath)}`),
  getRun: (id) => request('GET', `/api/runs/${id}`),
  cancelRun: (id) => request('POST', `/api/runs/${id}/cancel`),
}
