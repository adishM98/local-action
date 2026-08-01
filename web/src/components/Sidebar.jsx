import { useEffect, useRef, useState } from 'react'
import {
  LayoutDashboard,
  KeyRound,
  Search,
  ChevronDown,
  Star,
  Workflow,
  Rocket,
  ShieldCheck,
  FlaskConical,
  BookOpen,
  MoreHorizontal,
  PanelLeftClose,
  PanelLeftOpen,
  Package,
  GitBranch,
  Pencil,
} from 'lucide-react'
import { StatusCircle } from './StatusIcon.jsx'
import { lastRunByWorkflow, relativeTime } from '../format.js'
import { isPinned, togglePinned, getPinned } from '../pins.js'

const CATEGORY_ORDER = ['CI/Build', 'Security', 'Testing', 'Deployment', 'Docs', 'Other']
const CATEGORY_ICON = {
  'CI/Build': Workflow,
  Security: ShieldCheck,
  Testing: FlaskConical,
  Deployment: Rocket,
  Docs: BookOpen,
  Other: MoreHorizontal,
}

const STATUS_FILTERS = [
  { key: '', label: 'All' },
  { key: 'running', label: 'Running' },
  { key: 'failed', label: 'Failed' },
  { key: 'success', label: 'Success' },
  { key: 'never', label: 'Never run' },
]

function displayCategory(category) {
  return category.replace('/', ' / ')
}

const MIN_WIDTH = 220
const MAX_WIDTH = 520
const DEFAULT_WIDTH = 300
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

function groupByCategory(workflows) {
  const groups = new Map()
  for (const wf of workflows) {
    const category = wf.autoCategory || 'Other'
    if (!groups.has(category)) groups.set(category, [])
    groups.get(category).push(wf)
  }
  return CATEGORY_ORDER.filter((c) => groups.has(c)).map((c) => [c, groups.get(c)])
}

export default function Sidebar({ repoPath, onCommit, branch, workflows, scanState, view, onNavigate, runs }) {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [collapsed, setCollapsed] = useState({})
  const [, forceUpdate] = useState(0)
  const [width, setWidth] = useState(() => Number(localStorage.getItem('sidebarWidth')) || DEFAULT_WIDTH)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => localStorage.getItem('sidebarCollapsed') === 'true')
  const [editing, setEditing] = useState(!repoPath)
  const [draft, setDraft] = useState(repoPath)
  const searchRef = useRef(null)
  const resizingRef = useRef(false)
  const recentPaths = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')

  useEffect(() => {
    setDraft(repoPath)
    setEditing(!repoPath)
  }, [repoPath])

  function commitPath() {
    const path = draft.trim()
    setEditing(false)
    if (path && path !== repoPath) {
      rememberPath(path)
      onCommit(path)
    }
  }

  useEffect(() => {
    localStorage.setItem('sidebarCollapsed', String(sidebarCollapsed))
  }, [sidebarCollapsed])

  // Drag-to-resize: mousedown on the handle starts it, then document-level
  // listeners track the drag regardless of where the cursor ends up.
  useEffect(() => {
    function onMove(e) {
      if (!resizingRef.current) return
      setWidth(Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, e.clientX)))
    }
    function onUp() {
      if (!resizingRef.current) return
      resizingRef.current = false
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    return () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
  }, [])

  useEffect(() => {
    localStorage.setItem('sidebarWidth', String(width))
  }, [width])

  function startResize(e) {
    e.preventDefault()
    resizingRef.current = true
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
  }

  // ⌘K/Ctrl+K focuses search — the one keyboard-first affordance built so
  // far; the fuller shortcut system (R to run, a shortcuts panel, etc.) is
  // a separate, deliberately deferred round.
  useEffect(() => {
    function onKeyDown(e) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        searchRef.current?.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [])

  const lastRun = lastRunByWorkflow(runs)

  const byStatus = workflows.filter((wf) => {
    if (!statusFilter) return true
    const run = lastRun[wf.file]
    if (statusFilter === 'never') return !run
    return run?.status === statusFilter
  })
  const q = search.trim().toLowerCase()
  const matching = q ? byStatus.filter((wf) => wf.name.toLowerCase().includes(q)) : byStatus
  const groups = groupByCategory(matching)

  const pinnedFiles = getPinned(repoPath)
  const pinned = pinnedFiles.map((f) => workflows.find((w) => w.file === f)).filter(Boolean)

  function togglePin(e, file) {
    e.stopPropagation()
    togglePinned(repoPath, file)
    forceUpdate((v) => v + 1) // pin state lives in localStorage, not React state — force a re-render to reflect it
  }

  function WorkflowRow({ wf }) {
    const run = lastRun[wf.file]
    const active = view.name === 'runs' && view.workflowFile === wf.file
    return (
      <div className="sidebar__item-row">
        <button
          className={`sidebar__item${active ? ' active' : ''}`}
          onClick={() => onNavigate({ name: 'runs', workflowFile: wf.file })}
          title={wf.name}
        >
          <span className="sidebar__item--flat">
            {run ? <StatusCircle status={run.status} /> : <span className="status-circle status-circle--none" />}
            <span className="sidebar__item-text">
              <span className="sidebar__item-label">{wf.name}</span>
              {run && (
                <span className="sidebar__item-meta">
                  {run.branch && <span>{run.branch}</span>}
                  <span>{relativeTime(run.createdAt)}</span>
                </span>
              )}
            </span>
          </span>
        </button>
        <button
          className={`sidebar__pin-btn${isPinned(repoPath, wf.file) ? ' pinned' : ''}`}
          onClick={(e) => togglePin(e, wf.file)}
          title={isPinned(repoPath, wf.file) ? 'Unpin workflow' : 'Pin workflow'}
        >
          <Star size={12} fill={isPinned(repoPath, wf.file) ? 'currentColor' : 'none'} />
        </button>
      </div>
    )
  }

  if (sidebarCollapsed) {
    return (
      <nav className="sidebar sidebar--collapsed">
        <button className="sidebar__expand-btn" onClick={() => setSidebarCollapsed(false)} title="Expand sidebar">
          <PanelLeftOpen size={16} />
        </button>
      </nav>
    )
  }

  return (
    <nav className="sidebar" style={{ width }}>
      <div className="sidebar__resize-handle" onMouseDown={startResize} title="Drag to resize" />
      <div className="sidebar__fixed">
        <div className="sidebar__repo">
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
                  onBlur={commitPath}
                  onKeyDown={(e) => e.key === 'Enter' && commitPath()}
                  placeholder="/path/to/repo"
                  spellCheck={false}
                />
              </div>
            ) : (
              <div className="repo-header__text">
                <span className="repo-header__name">{repoNameFor(repoPath)}</span>
                <button
                  className="repo-header__meta"
                  onClick={() => setEditing(true)}
                  title={`${repoPath} — click to change`}
                >
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
          <button className="sidebar__collapse-btn" onClick={() => setSidebarCollapsed(true)} title="Collapse sidebar">
            <PanelLeftClose size={15} />
          </button>
        </div>
        <button
          className={`sidebar__item sidebar__item--flat sidebar__item--nav${view.name === 'overview' ? ' active' : ''}`}
          onClick={() => onNavigate({ name: 'overview' })}
        >
          <LayoutDashboard size={14} />
          Overview
        </button>
        <button
          className={`sidebar__item sidebar__item--flat sidebar__item--nav${view.name === 'secrets' ? ' active' : ''}`}
          onClick={() => onNavigate({ name: 'secrets', workflowFile: null })}
        >
          <KeyRound size={14} />
          Secrets &amp; variables
        </button>

        <div className="sidebar__search-wrap">
          <Search size={13} className="sidebar__search-icon" />
          <input
            ref={searchRef}
            className="sidebar__search"
            placeholder="Search workflows…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <span className="sidebar__search-hint">⌘K</span>
        </div>
        {workflows.length > 0 && (
          <button className="sidebar__filter-toggle" onClick={() => setFiltersOpen((v) => !v)}>
            Filter{statusFilter && `: ${STATUS_FILTERS.find((f) => f.key === statusFilter)?.label}`}
            <ChevronDown size={12} className={filtersOpen ? 'sidebar__filter-chevron--open' : ''} />
          </button>
        )}
        {filtersOpen && (
          <div className="sidebar__filters">
            {STATUS_FILTERS.map((f, i) => (
              <span key={f.key}>
                {i > 0 && <span className="sidebar__filters-sep">•</span>}
                <button
                  className={`sidebar__filter${statusFilter === f.key ? ' active' : ''}`}
                  onClick={() => {
                    setStatusFilter(f.key)
                    setFiltersOpen(false)
                  }}
                >
                  {f.label}
                </button>
              </span>
            ))}
          </div>
        )}

        {pinned.length > 0 && (
          <>
            <div className="sidebar__heading">
              <Star size={11} fill="currentColor" style={{ display: 'inline', verticalAlign: '-1px', marginRight: 4 }} />
              Favorites
            </div>
            {pinned.map((wf) => (
              <WorkflowRow wf={wf} key={wf.file} />
            ))}
          </>
        )}

        <div className="sidebar__divider" />
      </div>

      <div className="sidebar__scroll">
        <button className="sidebar__heading sidebar__heading--link" onClick={() => onNavigate({ name: 'runs', workflowFile: null })}>
          Workflow Explorer
        </button>
        {q && matching.length === 0 && <p className="sidebar__note">No workflows match "{search}".</p>}
        {groups.map(([category, items]) => {
          const Icon = CATEGORY_ICON[category] || MoreHorizontal
          const isCollapsed = collapsed[category]
          return (
            <div className="sidebar__group" key={category}>
              <button
                className="sidebar__group-heading"
                onClick={() => setCollapsed((c) => ({ ...c, [category]: !c[category] }))}
                aria-expanded={!isCollapsed}
              >
                <span className="sidebar__group-chevron">{isCollapsed ? '▸' : '▾'}</span>
                <span className="sidebar__group-icon">
                  <Icon size={13} />
                </span>
                {displayCategory(category)}
                <span className="sidebar__group-count">({items.length})</span>
              </button>
              {!isCollapsed && items.map((wf) => <WorkflowRow wf={wf} key={wf.file} />)}
            </div>
          )
        })}
        {scanState.error && <p className="sidebar__note sidebar__note--error">{scanState.error}</p>}
        {scanState.scanned && !scanState.error && workflows.length === 0 && (
          <p className="sidebar__note">No workflows under .github/workflows.</p>
        )}
      </div>
    </nav>
  )
}
