import { useEffect, useState } from 'react'
import { api } from '../api.js'

export default function SecretsPanel({ repoPath }) {
  const [kind, setKind] = useState('secret')
  const [entries, setEntries] = useState([])
  const [name, setName] = useState('')
  const [value, setValue] = useState('')

  async function load() {
    if (!repoPath) return
    setEntries(await api.listSecrets(repoPath, kind))
  }

  useEffect(() => {
    load()
  }, [repoPath, kind])

  async function save() {
    await api.upsertSecret(repoPath, kind, name, value)
    setName('')
    setValue('')
    load()
  }

  async function remove(key) {
    await api.deleteSecret(repoPath, kind, key)
    load()
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
      <ul>
        {entries.map((entry) => (
          <li key={entry.key}>
            {entry.key} <button onClick={() => remove(entry.key)}>Delete</button>
          </li>
        ))}
      </ul>
      <div className="row">
        <input placeholder="KEY" value={name} onChange={(e) => setName(e.target.value)} />
        <input placeholder="value" value={value} onChange={(e) => setValue(e.target.value)} />
        <button onClick={save} disabled={!name}>
          Save
        </button>
      </div>
    </div>
  )
}
