import { useEffect, useMemo } from 'react'
import { ReactFlow, Background, Controls, Handle, Position, useEdgesState, useNodesState } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import StatusIcon from './StatusIcon.jsx'
import { liveStatus } from '../logparse.js'
import { computeGraphLevels } from '../graph.js'

const COLUMN_GAP = 220
const ROW_GAP = 64

function JobNode({ data }) {
  return (
    <div className={`job-graph__node job-graph__node--${data.status || 'queued'}`}>
      <Handle type="target" position={Position.Left} className="job-graph__handle" />
      <StatusIcon status={data.status} />
      <span className="job-graph__node-name">{data.name}</span>
      <Handle type="source" position={Position.Right} className="job-graph__handle" />
    </div>
  )
}

const nodeTypes = { job: JobNode }

// Topological levels give each job a starting column/row; React Flow owns
// position from then on, so a user's drag survives future status updates —
// only a genuine job-set change (new run selected) resets the layout.
function buildLayout(jobs) {
  const byId = new Map(jobs.map((j) => [j.id, j]))
  const levels = computeGraphLevels(jobs)
  const positions = new Map()
  levels.forEach((columnIds, col) => {
    columnIds.forEach((id, row) => positions.set(id, { x: col * COLUMN_GAP, y: row * ROW_GAP }))
  })
  const nodes = jobs.map((job) => ({
    id: job.id,
    type: 'job',
    position: positions.get(job.id) || { x: 0, y: 0 },
    data: { name: job.name },
  }))
  const edges = []
  jobs.forEach((job) => {
    ;(job.needs || []).forEach((needId) => {
      if (!byId.has(needId)) return
      edges.push({ id: `${needId}->${job.id}`, source: needId, target: job.id })
    })
  })
  return { nodes, edges }
}

export default function JobGraph({ jobs, runtimeJobsById, runStatus, onSelectJob }) {
  const initial = useMemo(() => buildLayout(jobs), [jobs])
  const [nodes, setNodes, onNodesChange] = useNodesState(initial.nodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initial.edges)

  useEffect(() => {
    setNodes(initial.nodes)
    setEdges(initial.edges)
  }, [initial, setNodes, setEdges])

  useEffect(() => {
    setNodes((nds) =>
      nds.map((n) => ({ ...n, data: { ...n.data, status: liveStatus(runtimeJobsById.get(n.id)?.result, runStatus) } }))
    )
  }, [runtimeJobsById, runStatus, setNodes])

  return (
    <div className="job-graph">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={(_, node) => onSelectJob(node.id)}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ maxZoom: 1, padding: 0.3 }}
        elementsSelectable={false}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={18} size={1} color="var(--border)" />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  )
}
