import { useCallback, useEffect, useState } from 'react'
import { api } from './api.js'
import TopBar from './components/TopBar.jsx'
import Sidebar from './components/Sidebar.jsx'
import RunsView from './components/RunsView.jsx'
import RunDetail from './components/RunDetail.jsx'
import SecretsPage from './components/SecretsPage.jsx'

export default function App() {
  const [repoPath, setRepoPath] = useState(localStorage.getItem('repoPath') || '')
  const [workflows, setWorkflows] = useState([])
  const [categories, setCategories] = useState({})
  const [scanState, setScanState] = useState({ scanned: false, error: null })
  const [view, setView] = useState({ name: 'runs', workflowFile: null })
  const [health, setHealth] = useState(null)

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
    try {
      const result = await api.scan(path)
      setWorkflows(result || [])
      setScanState({ scanned: true, error: null })
    } catch (err) {
      setWorkflows([])
      setScanState({ scanned: true, error: err.message })
    }
    try {
      setCategories((await api.getWorkflowCategories(path)) || {})
    } catch {
      setCategories({})
    }
  }, [])

  useEffect(() => {
    scan(repoPath)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []) // initial scan for the remembered path only; later scans go through commitRepoPath

  function commitRepoPath(path) {
    setRepoPath(path)
    localStorage.setItem('repoPath', path)
    setView({ name: 'runs', workflowFile: null })
    scan(path)
  }

  return (
    <div className="app">
      <TopBar repoPath={repoPath} onCommit={commitRepoPath} health={health} onRecheck={checkHealth} />
      <div className="shell">
        <Sidebar
          workflows={workflows}
          scanState={scanState}
          view={view}
          onNavigate={setView}
          repoPath={repoPath}
          categories={categories}
          onCategoryChange={(file, category) =>
            setCategories((prev) => {
              const next = { ...prev }
              if (category) next[file] = category
              else delete next[file]
              return next
            })
          }
        />
        <main className="content">
          {view.name === 'runs' && (
            <RunsView
              repoPath={repoPath}
              workflows={workflows}
              workflowFile={view.workflowFile}
              health={health}
              onOpenRun={(runId) => setView({ name: 'run', runId, workflowFile: view.workflowFile })}
              onOpenSecrets={(workflowFile) => setView({ name: 'secrets', workflowFile })}
            />
          )}
          {view.name === 'run' && (
            <RunDetail
              runId={view.runId}
              onBack={() => setView({ name: 'runs', workflowFile: view.workflowFile || null })}
              onOpenRun={(runId) => setView({ name: 'run', runId, workflowFile: view.workflowFile })}
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
    </div>
  )
}
