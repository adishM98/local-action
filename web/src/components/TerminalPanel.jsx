import { useEffect, useRef, useState } from 'react'
import { Plus, X, ChevronDown, SquareTerminal } from 'lucide-react'
import { api } from '../api.js'
import TerminalView from './TerminalView.jsx'

const OPEN_KEY = 'terminalPanelOpen'

// Mounted once at the app root so shells keep running across navigation —
// closing the panel just hides it, it doesn't kill anything server-side.
export default function TerminalPanel({ repoPath }) {
  const [open, setOpen] = useState(() => localStorage.getItem(OPEN_KEY) === '1')
  const [tabs, setTabs] = useState([])
  const [activeId, setActiveId] = useState(null)
  const restoredForRef = useRef(null)

  useEffect(() => {
    localStorage.setItem(OPEN_KEY, open ? '1' : '0')
  }, [open])

  // Restore whatever sessions are still alive for this repo — covers a page
  // refresh (the panel remounts, but the shells never stopped) and a repo
  // switch (previous repo's tabs shouldn't show up here). repoPath can be
  // "" (no repo selected yet) — that's still a valid, distinct scope.
  useEffect(() => {
    if (restoredForRef.current === repoPath) return
    restoredForRef.current = repoPath
    setTabs([])
    setActiveId(null)
    api
      .listTerminalSessions(repoPath)
      .then((ids) => {
        setTabs(ids)
        setActiveId(ids[0] || null)
      })
      .catch(() => {})
  }, [repoPath])

  useEffect(() => {
    function onKeyDown(e) {
      if (e.ctrlKey && e.key === '`') {
        e.preventDefault()
        setOpen((o) => !o)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  async function newTab() {
    const { id } = await api.createTerminalSession(repoPath)
    setTabs((t) => [...t, id])
    setActiveId(id)
    setOpen(true)
  }

  function closeTab(id) {
    setTabs((t) => {
      const next = t.filter((x) => x !== id)
      setActiveId((cur) => (cur === id ? next[next.length - 1] || null : cur))
      return next
    })
    api.killTerminalSession(id).catch(() => {})
  }

  return (
    <div className={`terminal-panel${open ? ' terminal-panel--open' : ''}`}>
      <div className="terminal-panel__bar">
        <button className="terminal-panel__toggle" onClick={() => setOpen((o) => !o)} title="Toggle terminal (Ctrl+`)">
          <SquareTerminal size={14} />
          Terminal
        </button>
        {open && (
          <div className="terminal-panel__tabs">
            {tabs.map((id, i) => (
              <div key={id} className={`terminal-panel__tab${id === activeId ? ' terminal-panel__tab--active' : ''}`}>
                <button className="terminal-panel__tab-label" onClick={() => setActiveId(id)}>
                  {i + 1}
                </button>
                <button className="terminal-panel__tab-close" onClick={() => closeTab(id)} title="Close terminal">
                  <X size={11} />
                </button>
              </div>
            ))}
            <button className="terminal-panel__new" onClick={newTab} title="New terminal">
              <Plus size={14} />
            </button>
          </div>
        )}
        {open && (
          <button className="terminal-panel__collapse" onClick={() => setOpen(false)} title="Hide panel">
            <ChevronDown size={14} />
          </button>
        )}
      </div>
      {open && (
        <div className="terminal-panel__body">
          {tabs.length === 0 ? (
            <div className="terminal-panel__empty">
              <p>No terminal running.</p>
              <button className="btn btn--primary" onClick={newTab}>
                <Plus size={14} /> New terminal
              </button>
            </div>
          ) : (
            tabs.map((id) => <TerminalView key={id} sessionId={id} active={id === activeId} />)
          )}
        </div>
      )}
    </div>
  )
}
