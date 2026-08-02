import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

// One xterm instance + its WebSocket, wired to a server-side pty session.
// Mounted once per tab and kept alive (just hidden) while inactive, so the
// socket doesn't reconnect and scrollback isn't lost on a tab switch.
export default function TerminalView({ sessionId, active }) {
  const containerRef = useRef(null)
  const termRef = useRef(null)
  const fitRef = useRef(null)

  useEffect(() => {
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: "'JetBrains Mono', monospace",
      theme: { background: 'transparent' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(containerRef.current)
    fit.fit()
    termRef.current = term
    fitRef.current = fit

    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/terminal/${sessionId}`)
    socket.binaryType = 'arraybuffer'

    function sendResize() {
      if (socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }

    socket.onopen = sendResize

    // The first message on reconnect is replayed history (may be a
    // reconnect to an already-running shell). Replayed bytes can contain
    // escape-sequence queries (cursor position, device attributes) that
    // xterm auto-answers via onData — but the shell already got its answer
    // the first time around, so a second one just lands as garbage
    // keystrokes on the prompt. Hold off forwarding onData until that first
    // (replay) chunk has fully rendered; a short fallback timer covers
    // sessions with no backlog to replay.
    let inputEnabled = false
    let firstMessage = true
    const enableInput = () => {
      inputEnabled = true
    }
    const fallback = setTimeout(enableInput, 500)

    socket.onmessage = (e) => {
      const data = new Uint8Array(e.data)
      if (firstMessage) {
        firstMessage = false
        term.write(data, enableInput)
      } else {
        term.write(data)
      }
    }

    const dataSub = term.onData((data) => {
      if (inputEnabled && socket.readyState === WebSocket.OPEN) socket.send(new TextEncoder().encode(data))
    })
    const resizeSub = term.onResize(sendResize)

    const observer = new ResizeObserver(() => fit.fit())
    observer.observe(containerRef.current)

    return () => {
      clearTimeout(fallback)
      observer.disconnect()
      dataSub.dispose()
      resizeSub.dispose()
      socket.close()
      term.dispose()
    }
  }, [sessionId])

  useEffect(() => {
    if (active) {
      fitRef.current?.fit()
      termRef.current?.focus()
    }
  }, [active])

  return <div className="terminal-view" ref={containerRef} style={{ display: active ? 'block' : 'none' }} />
}
