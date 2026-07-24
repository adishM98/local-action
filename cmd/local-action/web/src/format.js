function unwrap(nullable) {
  if (nullable == null) return null
  if (typeof nullable === 'number') return nullable
  return nullable.Valid ? nullable.Int64 : null
}

export function relativeTime(unixSeconds) {
  if (!unixSeconds) return ''
  const delta = Math.floor(Date.now() / 1000) - unixSeconds
  if (delta < 60) return 'just now'
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`
  return `${Math.floor(delta / 86400)}d ago`
}

export function duration(run) {
  const start = unwrap(run.startedAt)
  if (!start) return ''
  const end = unwrap(run.finishedAt) ?? Math.floor(Date.now() / 1000)
  const secs = Math.max(0, end - start)
  const m = Math.floor(secs / 60)
  const s = secs % 60
  return m ? `${m}m ${s}s` : `${s}s`
}
