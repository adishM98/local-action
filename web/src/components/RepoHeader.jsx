import { useEffect, useState } from 'react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { Sun, Moon, Monitor, Check } from 'lucide-react'
import HealthItem from './HealthItem.jsx'

const THEME_KEY = 'theme'

const THEME_MODES = [
  { key: 'system', label: 'System', icon: Monitor },
  { key: 'light', label: 'Light', icon: Sun },
  { key: 'dark', label: 'Dark', icon: Moon },
]

function systemPrefersLight() {
  return window.matchMedia?.('(prefers-color-scheme: light)').matches
}

function resolveEffective(mode) {
  return mode === 'system' ? (systemPrefersLight() ? 'light' : 'dark') : mode
}

// mode is the user's explicit choice ('system' | 'light' | 'dark'), always
// persisted. effective is what's actually rendered — for 'system' that
// tracks the OS preference live (listens for prefers-color-scheme changes
// so the picker's icon doesn't go stale if you flip your OS theme while
// this tab is open; the CSS itself already reacts live via @media, no JS
// needed there — this listener is only to keep the icon in sync with it).
function useTheme() {
  const [mode, setMode] = useState(() => localStorage.getItem(THEME_KEY) || 'system')
  const [effective, setEffective] = useState(() => resolveEffective(mode))

  useEffect(() => {
    localStorage.setItem(THEME_KEY, mode)
    if (mode === 'system') {
      delete document.documentElement.dataset.theme
    } else {
      document.documentElement.dataset.theme = mode
    }
    setEffective(resolveEffective(mode))
  }, [mode])

  useEffect(() => {
    if (mode !== 'system' || !window.matchMedia) return
    const mq = window.matchMedia('(prefers-color-scheme: light)')
    const onChange = () => setEffective(resolveEffective('system'))
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [mode])

  return [mode, effective, setMode]
}

// Pure app chrome — brand + Docker/act health + theme. Repo identity (path,
// branch) lives on the Overview page now: this bar is "the app," Overview
// is "the repo you're pointed at" — two different concepts, kept visually
// separate instead of mixed into one header.
export default function RepoHeader({ health, onRecheck, onNavigate }) {
  const [mode, effective, setMode] = useTheme()
  const ThemeIcon = effective === 'light' ? Sun : Moon

  return (
    <header className="repo-header">
      <button className="repo-header__brand" onClick={() => onNavigate({ name: 'overview' })} title="Go to Overview">
        <img src="/logo.svg" alt="local-action" className="repo-header__brand-logo" />
        <span className="repo-header__brand-text">
          <span className="repo-header__brand-name">
            local<span className="repo-header__brand-accent">-action</span>
          </span>
          {import.meta.env.DEV && <span className="repo-header__brand-tagline">Run GitHub Actions locally</span>}
        </span>
      </button>
      <div className="health">
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button className="health__item" title={`Theme: ${mode}`}>
              <ThemeIcon size={18} />
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content className="run-row__overflow-menu" align="end" sideOffset={6}>
              {THEME_MODES.map((m) => (
                <DropdownMenu.Item asChild key={m.key}>
                  <button className="theme-menu__item" onClick={() => setMode(m.key)}>
                    <m.icon size={14} />
                    <span className="theme-menu__label">{m.label}</span>
                    {mode === m.key && <Check size={13} />}
                  </button>
                </DropdownMenu.Item>
              ))}
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
        <HealthItem label="Docker" ok={health?.dockerOK} error={health?.dockerError} onClick={onRecheck} />
      </div>
    </header>
  )
}
