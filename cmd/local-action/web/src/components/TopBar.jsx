import { useState } from 'react'

const RECENT_KEY = 'recentRepoPaths'
const MAX_RECENT = 8

function rememberPath(path) {
  if (!path) return
  const existing = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
  const next = [path, ...existing.filter((p) => p !== path)].slice(0, MAX_RECENT)
  localStorage.setItem(RECENT_KEY, JSON.stringify(next))
}

function HealthDot({ label, ok, error, onClick }) {
  const state = ok == null ? 'pending' : ok ? 'ok' : 'bad'
  const title =
    ok == null
      ? `checking ${label}…`
      : ok
        ? `${label} ready — click to recheck`
        : `${label} not available${error ? `: ${error}` : ''} — click to recheck`
  return (
    <button className="health__item" title={title} onClick={onClick}>
      <span className={`dot dot--${state}`} />
      {label}
    </button>
  )
}

export default function TopBar({ repoPath, onCommit, health, onRecheck }) {
  const [draft, setDraft] = useState(repoPath)
  const [focused, setFocused] = useState(false)
  const recentPaths = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')

  function commit() {
    const path = draft.trim()
    if (path && path !== repoPath) {
      rememberPath(path)
      onCommit(path)
    }
  }

  return (
    <header className="top-bar">
      <span className="top-bar__logo">local-action:~$</span>
      <div className="top-bar__path-wrap">
        <input
          className="top-bar__path"
          list="recent-repo-paths"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onFocus={() => setFocused(true)}
          onBlur={() => {
            setFocused(false)
            commit()
          }}
          onKeyDown={(e) => e.key === 'Enter' && commit()}
          placeholder="/path/to/repo"
          spellCheck={false}
        />
        {focused && <span className="top-bar__cursor" aria-hidden="true" />}
      </div>
      <datalist id="recent-repo-paths">
        {recentPaths.map((p) => (
          <option key={p} value={p} />
        ))}
      </datalist>
      <div className="health">
        <HealthDot label="act" ok={health?.actOK} onClick={onRecheck} />
        <HealthDot label="docker" ok={health?.dockerOK} error={health?.dockerError} onClick={onRecheck} />
      </div>
    </header>
  )
}
