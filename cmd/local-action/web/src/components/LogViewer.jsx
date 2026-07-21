import { useEffect, useState } from 'react'
import { api } from '../api.js'

export default function LogViewer({ runId, onCancel }) {
  const [lines, setLines] = useState([])
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState(null)
  const [cancelling, setCancelling] = useState(false)

  useEffect(() => {
    setLines([])
    setError(null)
    let cancelled = false

    // Persisted logs are the source of truth for anything that already
    // finished (the WS hub forgets its in-memory buffer once a run reaches
    // a terminal state). For runs still in progress, skip seeding here and
    // let the WS connection below replay its buffer + stream live, so we
    // don't show every line twice.
    api
      .getRun(runId)
      .then((result) => {
        if (cancelled) return
        const status = result?.run?.status
        if (status && status !== 'running' && status !== 'queued') {
          setLines(result.logs || [])
        }
      })
      .catch((err) => {
        if (!cancelled) setError(`Couldn't load this run: ${err.message}`)
      })

    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/runs/${runId}`)
    socket.onmessage = (event) => {
      setLines((prev) => [...prev, event.data])
    }
    socket.onerror = () => {
      if (!cancelled) setError('Lost the live log connection — showing what was received so far.')
    }
    return () => {
      cancelled = true
      socket.close()
    }
  }, [runId])

  async function copyLog() {
    try {
      await navigator.clipboard.writeText(lines.join('\n'))
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch (err) {
      setError(`Couldn't copy the log: ${err.message}`)
    }
  }

  async function cancel() {
    setCancelling(true)
    setError(null)
    try {
      await onCancel()
    } catch (err) {
      setError(`Couldn't cancel: ${err.message}`)
    } finally {
      setCancelling(false)
    }
  }

  return (
    <div className="log-viewer">
      <div className="row">
        <h3>Run #{runId}</h3>
        <button className="icon-btn" onClick={copyLog} disabled={!lines.length} title="Copy full log">
          {copied ? '✓ Copied' : '⧉ Copy log'}
        </button>
        <button className="ghost" onClick={cancel} disabled={cancelling}>
          {cancelling ? 'Cancelling…' : 'Cancel'}
        </button>
      </div>
      {error && <p className="error">{error}</p>}
      <pre>{lines.length ? lines.join('\n') : 'Waiting for output...'}</pre>
    </div>
  )
}
