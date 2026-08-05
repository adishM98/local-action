import { useEffect, useRef, useState } from 'react'
import { Lock, Unlock, TriangleAlert } from 'lucide-react'
import { api } from '../api.js'

export default function SecretsPage({ repoPath, workflows, initialWorkflowFilter }) {
  const [kind, setKind] = useState('secret')
  const [entries, setEntries] = useState([])
  const [filter, setFilter] = useState(initialWorkflowFilter || '')
  const [name, setName] = useState('')
  const [value, setValue] = useState('')
  const [scope, setScope] = useState(initialWorkflowFilter || '')
  const [error, setError] = useState(null)
  const [editing, setEditing] = useState(null) // {key, workflowFile} | null
  const [revealed, setRevealed] = useState(false)
  const [revealable, setRevealable] = useState(true) // this entry's saved choice: viewable/editable later, or write-only
  const valueRef = useRef(null)

  async function load() {
    if (!repoPath) return
    try {
      setEntries((await api.listSecrets(repoPath, kind)) || [])
      setError(null)
    } catch (err) {
      setError(err.message)
    }
  }

  useEffect(() => {
    load()
    cancelEdit()
  }, [repoPath, kind])

  function cancelEdit() {
    setEditing(null)
    setName('')
    setValue('')
    setScope(initialWorkflowFilter || '')
    setRevealed(false)
    setRevealable(true)
  }

  // Name and scope are locked while editing: they're the identity
  // upsertSecret matches on, so changing either here would silently create
  // a second entry instead of updating this one. The write-only/revealable
  // choice itself stays editable — flipping it just changes what happens to
  // *this save*, not history: turning a revealable entry write-only from
  // here on is a legitimate tightening, and turning a write-only entry
  // revealable only ever exposes whatever new value you're about to type in
  // (its old value was never fetched, so there's nothing retroactive to leak).
  async function startEdit(entry) {
    setEditing({ key: entry.key, workflowFile: entry.workflowFile || '' })
    setName(entry.key)
    setScope(entry.workflowFile || '')
    setValue('')
    setError(null)
    setRevealed(false)
    setRevealable(entry.revealable)
    valueRef.current?.focus()
    if (!entry.revealable) return // write-only — nothing to fetch, by design
    try {
      const result = await api.getSecretValue(repoPath, kind, entry.key, entry.workflowFile || '')
      setValue(result?.value || '')
    } catch (err) {
      setError(`Couldn't load the current value: ${err.message}`)
    }
  }

  async function save() {
    setError(null)
    try {
      await api.upsertSecret(repoPath, kind, name, value, scope, kind === 'secret' ? revealable : true)
      cancelEdit()
      load()
    } catch (err) {
      setError(err.message)
    }
  }

  async function remove(entry) {
    setError(null)
    try {
      await api.deleteSecret(repoPath, kind, entry.key, entry.workflowFile || '')
      if (editing?.key === entry.key && editing.workflowFile === (entry.workflowFile || '')) cancelEdit()
      load()
    } catch (err) {
      setError(err.message)
    }
  }

  if (!repoPath) {
    return <p className="empty-state">Enter a repo path above to manage secrets.</p>
  }

  const noun = kind === 'secret' ? 'secret' : 'variable'
  const visible = filter ? entries.filter((e) => !e.workflowFile || e.workflowFile === filter) : entries
  const wfName = (file) => workflows.find((w) => w.file === file)?.name || file

  const filteredWorkflow = filter ? workflows.find((w) => w.file === filter) : null
  const detected = (kind === 'secret' ? filteredWorkflow?.usedSecrets : filteredWorkflow?.usedVars) || []
  const storedNames = new Set(
    entries.filter((e) => !e.workflowFile || e.workflowFile === filter).map((e) => e.key),
  )
  const chips = detected.filter((n) => !storedNames.has(n))

  function quickAdd(detectedName) {
    setEditing(null)
    setName(detectedName)
    setScope(filter)
    setValue('')
    setRevealed(false)
    setRevealable(true)
    valueRef.current?.focus()
  }

  return (
    <div className="secrets-page">
      <h2>Secrets and variables</h2>
      <div className="tabs">
        <button className={`tab${kind === 'secret' ? ' active' : ''}`} onClick={() => setKind('secret')}>
          Secrets
        </button>
        <button className={`tab${kind === 'var' ? ' active' : ''}`} onClick={() => setKind('var')}>
          Variables
        </button>
      </div>
      {workflows.length > 0 && (
        <div className="field-row">
          <label>Filter by workflow</label>
          <select value={filter} onChange={(e) => setFilter(e.target.value)}>
            <option value="">All</option>
            {workflows.map((w) => (
              <option key={w.file} value={w.file}>
                {w.name}
              </option>
            ))}
          </select>
        </div>
      )}
      {chips.length > 0 && (
        <div className="detected-chips">
          <span>Detected in this workflow — click to add:</span>
          <div className="detected-chips__row">
            {chips.map((n) => (
              <button key={n} className="chip" onClick={() => quickAdd(n)}>
                + {n}
              </button>
            ))}
          </div>
        </div>
      )}
      {error && <p className="error">{error}</p>}
      {visible.length === 0 ? (
        <p className="empty-state">No {noun}s stored yet.</p>
      ) : (
        <table className="secret-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Scope</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {visible.map((entry) => (
              <tr key={`${entry.key}|${entry.workflowFile}`}>
                <td>
                  {entry.key}
                  {kind === 'secret' && !entry.revealable && (
                    <span className="scope-badge scope-badge--write-only" title="Saved as write-only — the value can't be viewed again, only overwritten">
                      Write-only
                    </span>
                  )}
                </td>
                <td>
                  {entry.workflowFile ? (
                    <span className="scope-badge scope-badge--wf" title={entry.workflowFile}>
                      {wfName(entry.workflowFile)}
                    </span>
                  ) : (
                    <span className="scope-badge">Repository</span>
                  )}
                </td>
                <td className="secret-table__actions">
                  <button className="linklike" onClick={() => startEdit(entry)}>
                    Edit
                  </button>
                  <button className="linklike" onClick={() => remove(entry)}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <h3>{editing ? `Edit ${noun}` : `New ${noun}`}</h3>
      <div className="field">
        <span>{noun === 'secret' ? 'Secret name' : 'Variable name'}</span>
        <input
          placeholder={noun === 'secret' ? 'e.g. AWS_ACCESS_KEY_ID' : 'e.g. NODE_VERSION'}
          value={name}
          onChange={(e) => setName(e.target.value)}
          disabled={Boolean(editing)}
        />
      </div>
      {kind === 'secret' && (
        <div className="field">
          <span>Storage</span>
          <div className="storage-options">
            <label className={`storage-card${revealable ? ' storage-card--active' : ''}`}>
              <input type="radio" name="storage" checked={revealable} onChange={() => setRevealable(true)} />
              <Unlock size={16} className="storage-card__icon" />
              <span className="storage-card__body">
                <span className="storage-card__title">Editable</span>
                <span className="storage-card__desc">View and edit this secret later.</span>
              </span>
            </label>
            <label className={`storage-card storage-card--warn${!revealable ? ' storage-card--active' : ''}`}>
              <input type="radio" name="storage" checked={!revealable} onChange={() => setRevealable(false)} />
              <Lock size={16} className="storage-card__icon" />
              <span className="storage-card__body">
                <span className="storage-card__title">Write-only (GitHub-style)</span>
                <span className="storage-card__desc">
                  <span className="storage-card__warning">
                    <TriangleAlert size={12} /> The value cannot be viewed after saving.
                  </span>{' '}
                  You can only replace it.
                </span>
              </span>
            </label>
          </div>
        </div>
      )}
      <div className="field">
        <div className="field__label-row">
          <span>{noun === 'secret' ? 'Secret value' : 'Variable value'}</span>
          {kind === 'secret' && (
            <button type="button" className="linklike" onClick={() => setRevealed((r) => !r)}>
              {revealed ? 'Hide' : 'Show'}
            </button>
          )}
        </div>
        <textarea
          ref={valueRef}
          className={`secret-value-input${kind === 'secret' && !revealed ? ' secret-value-input--hidden' : ''}`}
          rows={3}
          placeholder={noun === 'secret' ? 'Paste your secret here…' : 'Enter the value…'}
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
        {kind === 'secret' && !revealed && (
          <small>Blurred on screen — click "Show" before a screen share, or if you just want to double-check it.</small>
        )}
      </div>
      <div className="field">
        <span>Scope</span>
        <label>
          <input type="radio" checked={scope === ''} onChange={() => setScope('')} disabled={Boolean(editing)} /> Entire
          repository
        </label>
        <label>
          <input
            type="radio"
            checked={scope !== ''}
            disabled={Boolean(editing) || workflows.length === 0}
            onChange={() => setScope(workflows[0]?.file || '')}
          />{' '}
          Only this workflow
        </label>
        {scope !== '' && (
          <select value={scope} onChange={(e) => setScope(e.target.value)} disabled={Boolean(editing)}>
            {workflows.map((w) => (
              <option key={w.file} value={w.file}>
                {w.name}
              </option>
            ))}
          </select>
        )}
      </div>
      <div className="secrets-form__actions">
        <button className="btn btn--primary" onClick={save} disabled={!name || (editing && !value)}>
          {editing ? `Update ${noun}` : `Save ${noun}`}
        </button>
        {editing && (
          <button className="btn" onClick={cancelEdit}>
            Cancel
          </button>
        )}
      </div>
    </div>
  )
}
