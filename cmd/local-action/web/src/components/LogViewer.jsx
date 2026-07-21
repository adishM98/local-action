import { useEffect, useRef, useState } from 'react'
import { api } from '../api.js'

export default function LogViewer({ runId, onCancel }) {
  const [lines, setLines] = useState([])
  const [copied, setCopied] = useState(false)
  const socketRef = useRef(null)

  useEffect(() => {
    setLines([])
    let cancelled = false

    // Persisted logs are the source of truth for anything that already
    // finished (the WS hub forgets its in-memory buffer once a run reaches
    // a terminal state). For runs still in progress, skip seeding here and
    // let the WS connection below replay its buffer + stream live, so we
    // don't show every line twice.
    api.getRun(runId).then((result) => {
      if (cancelled) return
      const status = result?.run?.status
      if (status && status !== 'running' && status !== 'queued') {
        setLines(result.logs || [])
      }
    })

    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/runs/${runId}`)
    socket.onmessage = (event) => {
      setLines((prev) => [...prev, event.data])
    }
    socketRef.current = socket
    return () => {
      cancelled = true
      socket.close()
    }
  }, [runId])

  async function copyLog() {
    await navigator.clipboard.writeText(lines.join('\n'))
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="log-viewer">
      <div className="row">
        <h3>Run #{runId}</h3>
        <button className="icon-btn" onClick={copyLog} disabled={!lines.length} title="Copy full log">
          {copied ? '✓ Copied' : '⧉ Copy log'}
        </button>
        <button className="ghost" onClick={onCancel}>
          Cancel
        </button>
      </div>
      <pre>{lines.length ? lines.join('\n') : 'Waiting for output...'}</pre>
    </div>
  )
}
