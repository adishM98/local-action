import { useEffect, useRef, useState } from 'react'

export default function LogViewer({ runId, onCancel }) {
  const [lines, setLines] = useState([])
  const socketRef = useRef(null)

  useEffect(() => {
    setLines([])
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/runs/${runId}`)
    socket.onmessage = (event) => {
      setLines((prev) => [...prev, event.data])
    }
    socketRef.current = socket
    return () => socket.close()
  }, [runId])

  return (
    <div className="log-viewer">
      <div className="row">
        <h3>Run #{runId}</h3>
        <button onClick={onCancel}>Cancel</button>
      </div>
      <pre>{lines.join('\n')}</pre>
    </div>
  )
}
