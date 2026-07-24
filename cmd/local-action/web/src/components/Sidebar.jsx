import { useState } from 'react'
import { api } from '../api.js'

const CATEGORY_ORDER = ['Deployment', 'Testing', 'Security', 'CI/Build', 'Docs', 'Other']
const CATEGORY_ICON = {
  Deployment: '🚀',
  Testing: '🧪',
  Security: '🔒',
  'CI/Build': '⚙️',
  Docs: '📚',
  Other: '📁',
}

function loadCollapsed() {
  try {
    return JSON.parse(localStorage.getItem('sidebarCollapsed') || '{}')
  } catch {
    return {}
  }
}

export default function Sidebar({ workflows, scanState, view, onNavigate, repoPath, categories, onCategoryChange }) {
  const [collapsed, setCollapsed] = useState(loadCollapsed)
  const [search, setSearch] = useState('')
  const inRuns = view.name === 'runs'

  function toggleCategory(name) {
    setCollapsed((prev) => {
      const next = { ...prev, [name]: !prev[name] }
      localStorage.setItem('sidebarCollapsed', JSON.stringify(next))
      return next
    })
  }

  async function changeCategory(wf, category) {
    onCategoryChange(wf.file, category)
    try {
      await api.saveWorkflowCategory(repoPath, wf.file, category)
    } catch {
      // best-effort — sidebar grouping is a convenience, not worth surfacing an error banner for
    }
  }

  const q = search.trim().toLowerCase()
  const matching = q ? workflows.filter((wf) => wf.name.toLowerCase().includes(q)) : workflows

  const groups = {}
  for (const wf of matching) {
    const effective = categories[wf.file] || wf.autoCategory || 'Other'
    ;(groups[effective] ||= []).push(wf)
  }

  return (
    <nav className="sidebar">
      <div className="sidebar__heading">Actions</div>
      {workflows.length > 5 && (
        <input
          className="sidebar__search"
          placeholder="Search workflows…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      )}
      <button
        className={`sidebar__item${inRuns && !view.workflowFile ? ' active' : ''}`}
        onClick={() => onNavigate({ name: 'runs', workflowFile: null })}
      >
        All workflows
      </button>
      {q && matching.length === 0 && <p className="sidebar__note">No workflows match "{search}".</p>}
      {CATEGORY_ORDER.filter((c) => groups[c]?.length).map((category) => (
        <div className="sidebar__group" key={category}>
          <button className="sidebar__group-heading" onClick={() => toggleCategory(category)}>
            <span className="sidebar__group-chevron">{collapsed[category] ? '▸' : '▾'}</span>
            <span className="sidebar__group-icon">{CATEGORY_ICON[category]}</span>
            {category}
          </button>
          {!collapsed[category] &&
            groups[category].map((wf) => (
              <div className="sidebar__item-row" key={wf.file}>
                <button
                  className={`sidebar__item${inRuns && view.workflowFile === wf.file ? ' active' : ''}`}
                  onClick={() => onNavigate({ name: 'runs', workflowFile: wf.file })}
                  title={wf.file}
                >
                  {wf.name}
                </button>
                <select
                  className="sidebar__category-select"
                  value={categories[wf.file] || ''}
                  onChange={(e) => changeCategory(wf, e.target.value)}
                  title="Change category"
                >
                  <option value="">Auto ({wf.autoCategory || 'Other'})</option>
                  {CATEGORY_ORDER.map((c) => (
                    <option key={c} value={c}>
                      {c}
                    </option>
                  ))}
                </select>
              </div>
            ))}
        </div>
      ))}
      {scanState.error && <p className="sidebar__note sidebar__note--error">{scanState.error}</p>}
      {scanState.scanned && !scanState.error && workflows.length === 0 && (
        <p className="sidebar__note">No workflows under .github/workflows.</p>
      )}
      <div className="sidebar__spacer" />
      <div className="sidebar__heading">Settings</div>
      <button
        className={`sidebar__item${view.name === 'secrets' ? ' active' : ''}`}
        onClick={() => onNavigate({ name: 'secrets', workflowFile: null })}
      >
        Secrets and variables
      </button>
    </nav>
  )
}
