import { useEffect, useState } from 'react'
import { api } from '../api.js'

export default function SecretsPanel({ repoPath }) {
  const [kind, setKind] = useState('secret')
  const [entries, setEntries] = useState([])
  const [name, setName] = useState('')
  const [value, setValue] = useState('')
  const [error, setError] = useState(null)

  async function load() {
    if (!repoPath) return
    try {
      setEntries(await api.listSecrets(repoPath, kind))
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
      await api.upsertSecret(repoPath, kind, name, value)
      setName('')
      setValue('')
      load()
    } catch (err) {
      setError(err.message)
    }
  }

  async function remove(key) {
    setError(null)
    try {
      await api.deleteSecret(repoPath, kind, key)
      load()
    } catch (err) {
      setError(err.message)
    }
  }

  if (!repoPath) {
    return <p className="empty-state">Enter a repo path above to manage its secrets.</p>
  }

  return (
    <div>
      <div className="row">
        <label>
          <input type="radio" checked={kind === 'secret'} onChange={() => setKind('secret')} /> Secrets
        </label>
        <label>
          <input type="radio" checked={kind === 'var'} onChange={() => setKind('var')} /> Vars
        </label>
      </div>
      {error && <p className="error">{error}</p>}
      {entries.length === 0 ? (
        <p className="empty-state">No {kind === 'secret' ? 'secrets' : 'vars'} stored for this repo yet.</p>
      ) : (
        <ul className="secret-list">
          {entries.map((entry) => (
            <li key={entry.key}>
              {entry.key}
              <button className="ghost" onClick={() => remove(entry.key)}>
                Delete
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="row">
        <input placeholder="KEY" value={name} onChange={(e) => setName(e.target.value)} />
        <input placeholder="value" value={value} onChange={(e) => setValue(e.target.value)} />
        <button className="primary" onClick={save} disabled={!name}>
          Save
        </button>
      </div>
    </div>
  )
}
