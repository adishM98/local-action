import { useState } from 'react'
import { api } from '../api.js'

export default function SettingsPage() {
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState(null)

  async function reset() {
    setBusy(true)
    setResult(null)
    try {
      await api.resetRunHistory()
      setResult({ ok: true, message: 'Run history cleared.' })
    } catch (err) {
      setResult({ ok: false, message: `Couldn't clear run history: ${err.message}` })
    } finally {
      setBusy(false)
      setConfirming(false)
    }
  }

  return (
    <div className="settings-page">
      <h2>Settings</h2>

      <section className="settings-section">
        <h3>Data</h3>
        <p className="settings-section__desc">
          Clears every run and its logs, across all repos you've pointed this app at. Secrets, workflow categories,
          and pinned workflows are untouched. This can't be undone.
        </p>
        {!confirming ? (
          <button className="btn" onClick={() => setConfirming(true)}>
            Reset run history
          </button>
        ) : (
          <div className="settings-confirm">
            <span>Clear all run history? This can't be undone.</span>
            <button className="btn" onClick={() => setConfirming(false)} disabled={busy}>
              Cancel
            </button>
            <button className="btn btn--danger" onClick={reset} disabled={busy}>
              {busy ? 'Clearing…' : 'Yes, clear it'}
            </button>
          </div>
        )}
        {result && <p className={result.ok ? 'settings-result' : 'error'}>{result.message}</p>}
      </section>
    </div>
  )
}
