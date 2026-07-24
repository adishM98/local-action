import { useEffect, useRef, useState } from 'react'
import { api } from '../api.js'

export default function SecretsPage({ repoPath, workflows, initialWorkflowFilter }) {
  const [kind, setKind] = useState('secret')
  const [entries, setEntries] = useState([])
  const [filter, setFilter] = useState(initialWorkflowFilter || '')
  const [name, setName] = useState('')
  const [value, setValue] = useState('')
  const [scope, setScope] = useState(initialWorkflowFilter || '')
  const [error, setError] = useState(null)
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
  }, [repoPath, kind])

  async function save() {
    setError(null)
    try {
      await api.upsertSecret(repoPath, kind, name, value, scope)
      setName('')
      setValue('')
      load()
    } catch (err) {
      setError(err.message)
    }
  }

  async function remove(entry) {
    setError(null)
    try {
      await api.deleteSecret(repoPath, kind, entry.key, entry.workflowFile || '')
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
    setName(detectedName)
    setScope(filter)
    setValue('')
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
                <td>{entry.key}</td>
                <td>
                  {entry.workflowFile ? (
                    <span className="scope-badge scope-badge--wf" title={entry.workflowFile}>
                      {wfName(entry.workflowFile)}
                    </span>
                  ) : (
                    <span className="scope-badge">Repository</span>
                  )}
                </td>
                <td>
                  <button className="linklike" onClick={() => remove(entry)}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <h3>New {noun}</h3>
      <div className="field">
        <span>Name</span>
        <input placeholder="KEY" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="field">
        <span>Value</span>
        <input
          ref={valueRef}
          placeholder="value (write-only after save)"
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
      </div>
      <div className="field">
        <span>Scope</span>
        <label>
          <input type="radio" checked={scope === ''} onChange={() => setScope('')} /> All workflows in this
          repo
        </label>
        <label>
          <input
            type="radio"
            checked={scope !== ''}
            disabled={workflows.length === 0}
            onChange={() => setScope(workflows[0]?.file || '')}
          />{' '}
          Specific workflow
        </label>
        {scope !== '' && (
          <select value={scope} onChange={(e) => setScope(e.target.value)}>
            {workflows.map((w) => (
              <option key={w.file} value={w.file}>
                {w.name}
              </option>
            ))}
          </select>
        )}
      </div>
      <button className="btn btn--primary" onClick={save} disabled={!name}>
        Add {noun}
      </button>
    </div>
  )
}
