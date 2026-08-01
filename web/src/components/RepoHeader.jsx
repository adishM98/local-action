import { useEffect, useState } from 'react'
import { Container, Terminal, Sun, Moon } from 'lucide-react'

const THEME_KEY = 'theme'

function systemPrefersLight() {
  return window.matchMedia?.('(prefers-color-scheme: light)').matches
}

// ponytail: two-state toggle (light/dark), no explicit "system" option in
// the UI — first click always pins an explicit choice. Good enough; add a
// three-way switch only if someone actually asks to get back to "follow OS".
function useTheme() {
  const [theme, setTheme] = useState(() => localStorage.getItem(THEME_KEY))

  useEffect(() => {
    if (theme) {
      document.documentElement.dataset.theme = theme
      localStorage.setItem(THEME_KEY, theme)
    } else {
      delete document.documentElement.dataset.theme
    }
  }, [theme])

  const effective = theme || (systemPrefersLight() ? 'light' : 'dark')
  const toggle = () => setTheme(effective === 'light' ? 'dark' : 'light')
  return [effective, toggle]
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

// Pure app chrome — brand + Docker/act health + theme. Repo identity (path,
// branch) lives in the sidebar now: this bar is "the app," the sidebar is
// "the repo you're pointed at" — two different concepts, kept visually
// separate instead of mixed into one header.
export default function RepoHeader({ health, onRecheck }) {
  const [theme, toggleTheme] = useTheme()
  const ThemeIcon = theme === 'light' ? Sun : Moon

  return (
    <header className="repo-header">
      <div className="repo-header__brand" title="local-action">
        <img src="/logo.png" alt="local-action" className="repo-header__brand-logo" />
        <span className="repo-header__brand-name">
          local<span className="repo-header__brand-accent">-action</span>
        </span>
      </div>
      <div className="health">
        <button
          className="health__item"
          onClick={toggleTheme}
          title={`Switch to ${theme === 'light' ? 'dark' : 'light'} theme`}
        >
          <ThemeIcon size={13} />
        </button>
        <HealthItem icon={Container} label="Docker" ok={health?.dockerOK} error={health?.dockerError} onClick={onRecheck} />
        <HealthItem icon={Terminal} label="Act" ok={health?.actOK} onClick={onRecheck} />
      </div>
    </header>
  )
}
