import { useCallback, useEffect, useState } from 'react'
import { api } from './api.js'
import RepoHeader from './components/RepoHeader.jsx'
import Sidebar from './components/Sidebar.jsx'
import Overview from './components/Overview.jsx'
import RunsView from './components/RunsView.jsx'
import RunDetail from './components/RunDetail.jsx'
import SecretsPage from './components/SecretsPage.jsx'
import Drawer from './components/Drawer.jsx'
import TerminalPanel from './components/TerminalPanel.jsx'
import VersionMigrationModal from './components/VersionMigrationModal.jsx'

function loadView() {
  try {
    const raw = localStorage.getItem('view')
    if (raw) return JSON.parse(raw)
  } catch {
    // corrupt/old value — fall through to the default below
  }
  return { name: 'overview' }
}

export default function App() {
  const [repoPath, setRepoPath] = useState(localStorage.getItem('repoPath') || '')
  const [workflows, setWorkflows] = useState([])
  const [scanState, setScanState] = useState({ scanned: false, error: null })
  const [scanning, setScanning] = useState(false)
  const [view, setView] = useState(loadView)
  const [health, setHealth] = useState(null)
  const [drawerRunId, setDrawerRunId] = useState(null)
  const [runs, setRuns] = useState([])
  const [runsError, setRunsError] = useState(null)
  const [branch, setBranch] = useState(null)
  const [versionMigration, setVersionMigration] = useState(null)

  useEffect(() => {
    api
      .getVersionMigration()
      .then((info) => {
        if (info.showPrompt) setVersionMigration(info)
      })
      .catch(() => {})
  }, [])

  function resolveVersionMigration(action) {
    api
      .resolveVersionMigration(action)
      .catch(() => {})
      .finally(() => setVersionMigration(null))
  }

  const checkHealth = useCallback(async () => {
    try {
      setHealth(await api.health())
    } catch {
      setHealth({ actOK: false, dockerOK: false, dockerError: 'server unreachable' })
    }
  }, [])

  useEffect(() => {
    checkHealth()
  }, [checkHealth])

  useEffect(() => {
    localStorage.setItem('view', JSON.stringify(view))
  }, [view])

  // Poll fast while unhealthy so a booting Docker Desktop turns the dot
  // green within seconds; back off once everything is fine.
  useEffect(() => {
    if (!health) return
    const healthy = health.actOK && health.dockerOK
    const id = setTimeout(checkHealth, healthy ? 30000 : 5000)
    return () => clearTimeout(id)
  }, [health, checkHealth])

  const scan = useCallback(async (path) => {
    if (!path) return
    setScanning(true)
    try {
      const result = await api.scan(path)
      setWorkflows(result || [])
      setScanState({ scanned: true, error: null })
    } catch (err) {
      setWorkflows([])
      setScanState({ scanned: true, error: err.message })
    } finally {
      setScanning(false)
    }
  }, [])

  useEffect(() => {
    scan(repoPath)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []) // initial scan for the remembered path only; later scans go through commitRepoPath

  // Repo header's branch line — refetched whenever the repo path changes.
  useEffect(() => {
    if (!repoPath) {
      setBranch(null)
      return
    }
    let cancelled = false
    api
      .repoInfo(repoPath)
      .then((result) => {
        if (!cancelled) setBranch(result?.branch || null)
      })
      .catch(() => {
        if (!cancelled) setBranch(null)
      })
    return () => {
      cancelled = true
    }
  }, [repoPath])

  // Polled here (not inside RunsView) so the sidebar's per-workflow
  // last-run-status icons share the same fetch instead of duplicating it.
  useEffect(() => {
    if (!repoPath) return
    let cancelled = false
    async function load() {
      try {
        const result = await api.listRuns(repoPath)
        if (!cancelled) {
          setRuns(result || [])
          setRunsError(null)
        }
      } catch (err) {
        if (!cancelled) setRunsError(err.message)
      }
    }
    load()
    const interval = setInterval(load, 2000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [repoPath])

  function commitRepoPath(path) {
    setRepoPath(path)
    localStorage.setItem('repoPath', path)
    setView({ name: 'overview' })
    scan(path)
  }

  return (
    <div className="app">
      <RepoHeader health={health} onRecheck={checkHealth} onNavigate={setView} />
      <div className="shell">
        <Sidebar
          repoPath={repoPath}
          workflows={workflows}
          scanState={scanState}
          scanning={scanning}
          onRescan={() => scan(repoPath)}
          view={view}
          onNavigate={setView}
          runs={runs}
        />
        <main className="content">
          {view.name === 'overview' && (
            <Overview
              repoPath={repoPath}
              onCommit={commitRepoPath}
              branch={branch}
              workflows={workflows}
              runs={runs}
              onOpenRun={setDrawerRunId}
              onNavigate={setView}
            />
          )}
          {view.name === 'runs' && (
            <RunsView
              repoPath={repoPath}
              workflows={workflows}
              workflowFile={view.workflowFile}
              health={health}
              runs={runs}
              runsError={runsError}
              onOpenRun={setDrawerRunId}
              onOpenSecrets={(workflowFile) => setView({ name: 'secrets', workflowFile })}
              onRescan={() => scan(repoPath)}
              scanning={scanning}
            />
          )}
          {view.name === 'secrets' && (
            <SecretsPage
              repoPath={repoPath}
              workflows={workflows}
              initialWorkflowFilter={view.workflowFile || ''}
            />
          )}
        </main>
      </div>
      {drawerRunId && (
        <Drawer onClose={() => setDrawerRunId(null)}>
          <RunDetail runId={drawerRunId} workflows={workflows} onClose={() => setDrawerRunId(null)} onOpenRun={setDrawerRunId} />
        </Drawer>
      )}
      <TerminalPanel repoPath={repoPath} />
      {versionMigration && (
        <VersionMigrationModal
          previousVersion={versionMigration.previousVersion}
          currentVersion={versionMigration.currentVersion}
          onResolve={resolveVersionMigration}
        />
      )}
    </div>
  )
}
