import { useEffect, useState } from 'react'
import { api } from '../api.js'

const RECENT_KEY = 'recentRepoPaths'
const MAX_RECENT = 8

export function rememberPath(path) {
  if (!path) return
  const existing = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
  const next = [path, ...existing.filter((p) => p !== path)].slice(0, MAX_RECENT)
  localStorage.setItem(RECENT_KEY, JSON.stringify(next))
}

function useHealth() {
  const [health, setHealth] = useState(null)

  useEffect(() => {
    let cancelled = false
    async function check() {
      try {
        const result = await api.health()
        if (!cancelled) setHealth(result)
      } catch {
        if (!cancelled) setHealth({ actOK: false, dockerOK: false })
      }
    }
    check()
    const interval = setInterval(check, 20000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [])

  return health
}

function HealthDot({ label, ok }) {
  const state = ok === undefined ? 'pending' : ok ? 'ok' : 'bad'
  const title =
    ok === undefined ? `checking ${label}...` : ok ? `${label} ready` : `${label} not available`
  return (
    <span className="health__item" title={title}>
      <span className={`dot dot--${state}`} />
      {label}
    </span>
  )
}

export default function PromptBar({ repoPath, setRepoPath }) {
  const [copied, setCopied] = useState(false)
  const health = useHealth()
  const recentPaths = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')

  async function copyPath() {
    if (!repoPath) return
    try {
      await navigator.clipboard.writeText(repoPath)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch (err) {
      console.error('copy repo path failed:', err)
    }
  }

  return (
    <div className="prompt-bar">
      <span className="prompt-bar__glyph">local-action:~$</span>
      <div className="prompt-bar__path-wrap">
        <input
          className="prompt-bar__path"
          list="recent-repo-paths"
          value={repoPath}
          onChange={(e) => setRepoPath(e.target.value)}
          placeholder="/path/to/repo"
          spellCheck={false}
        />
        <datalist id="recent-repo-paths">
          {recentPaths.map((p) => (
            <option key={p} value={p} />
          ))}
        </datalist>
        <button
          className={`icon-btn${copied ? ' copied' : ''}`}
          onClick={copyPath}
          disabled={!repoPath}
          title="Copy repo path"
          aria-label="Copy repo path"
        >
          {copied ? '✓' : '⧉'}
        </button>
      </div>
      <div className="health">
        <HealthDot label="act" ok={health?.actOK} />
        <HealthDot label="docker" ok={health?.dockerOK} />
      </div>
    </div>
  )
}
