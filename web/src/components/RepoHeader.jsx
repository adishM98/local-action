import { useEffect, useState } from 'react'
import { Package, GitBranch, Container, Terminal, Pencil } from 'lucide-react'

const RECENT_KEY = 'recentRepoPaths'
const MAX_RECENT = 8

function rememberPath(path) {
  if (!path) return
  const existing = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
  const next = [path, ...existing.filter((p) => p !== path)].slice(0, MAX_RECENT)
  localStorage.setItem(RECENT_KEY, JSON.stringify(next))
}

function repoNameFor(path) {
  return path ? path.split('/').filter(Boolean).pop() || path : ''
}

function HealthItem({ icon: Icon, label, ok, error, onClick }) {
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
      <Icon size={13} />
      {label}
    </button>
  )
}

export default function RepoHeader({ repoPath, onCommit, health, onRecheck, branch }) {
  const [editing, setEditing] = useState(!repoPath)
  const [draft, setDraft] = useState(repoPath)
  const [focused, setFocused] = useState(false)
  const recentPaths = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')

  useEffect(() => {
    setDraft(repoPath)
    setEditing(!repoPath)
  }, [repoPath])

  function commit() {
    const path = draft.trim()
    setEditing(false)
    if (path && path !== repoPath) {
      rememberPath(path)
      onCommit(path)
    }
  }

  return (
    <header className="repo-header">
      <div className="repo-header__identity">
        <span className="repo-header__icon">
          <Package size={18} />
        </span>
        {editing ? (
          <div className="repo-header__path-wrap">
            <input
              className="repo-header__path-input"
              list="recent-repo-paths"
              autoFocus={!!repoPath}
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
            {focused && <span className="repo-header__cursor" aria-hidden="true" />}
          </div>
        ) : (
          <div className="repo-header__text">
            <span className="repo-header__name">{repoNameFor(repoPath)}</span>
            <button className="repo-header__meta" onClick={() => setEditing(true)} title={`${repoPath} — click to change`}>
              {branch ? (
                <>
                  <span className="repo-header__meta-icon">
                    <GitBranch size={14} strokeWidth={2.25} />
                  </span>
                  {branch}
                </>
              ) : (
                <span className="repo-header__meta-path">{repoPath}</span>
              )}
              <Pencil size={11} className="repo-header__meta-edit" />
            </button>
          </div>
        )}
      </div>
      <datalist id="recent-repo-paths">
        {recentPaths.map((p) => (
          <option key={p} value={p} />
        ))}
      </datalist>
      <div className="health">
        <HealthItem icon={Container} label="Docker" ok={health?.dockerOK} error={health?.dockerError} onClick={onRecheck} />
        <HealthItem icon={Terminal} label="Act" ok={health?.actOK} onClick={onRecheck} />
      </div>
    </header>
  )
}
