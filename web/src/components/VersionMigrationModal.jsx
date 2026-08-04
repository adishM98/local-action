import { useEffect } from 'react'

// Shown once per version bump (see /api/version-migration) — Escape acts
// like "Keep", the same safe default as the missing close button: this
// modal exists to ask a real either/or question, not to be dismissed
// without an answer.
export default function VersionMigrationModal({ previousVersion, currentVersion, onResolve }) {
  useEffect(() => {
    function onKeyDown(e) {
      if (e.key === 'Escape') onResolve('keep')
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onResolve])

  return (
    <div className="modal-backdrop">
      <div className="modal">
        <h3>Updated to v{currentVersion}</h3>
        <p>
          You were on v{previousVersion}. Keep your existing run history, or start fresh with this version?
        </p>
        <div className="modal__actions">
          <button className="btn" onClick={() => onResolve('clear')}>
            Clear run history
          </button>
          <button className="btn btn--primary" onClick={() => onResolve('keep')}>
            Keep run history
          </button>
        </div>
      </div>
    </div>
  )
}
