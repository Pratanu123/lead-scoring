import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, connectLeadSocket, type Lead, type LeadScore, type SimilarLead } from '../api'

const statuses = ['new', 'contacted', 'qualified', 'won', 'lost', 'disqualified']

export default function LeadDetailPage() {
  const { id = '' } = useParams()
  const [lead, setLead] = useState<Lead | null>(null)
  const [score, setScore] = useState<LeadScore | null>(null)
  const [scores, setScores] = useState<LeadScore[]>([])
  const [similar, setSimilar] = useState<SimilarLead[]>([])
  const [jobStatus, setJobStatus] = useState('')
  const [error, setError] = useState('')
  const [scoring, setScoring] = useState(false)

  const probability = useMemo(
    () => (score ? `${Math.round(score.conversion_probability * 100)}%` : '—'),
    [score],
  )

  async function refresh() {
    if (!id) return
    setError('')
    try {
      const [leadData, similarData, scoreHistory] = await Promise.all([
        api.getLead(id),
        api.similar(id).catch(() => []),
        api.listScores(id).catch(() => []),
      ])
      setLead(leadData)
      setSimilar(similarData)
      setScores(scoreHistory)
      try {
        setScore(await api.latestScore(id))
      } catch {
        setScore(null)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load lead')
    }
  }

  useEffect(() => {
    void refresh()
  }, [id])

  useEffect(() => {
    if (!id) return
    const socket = connectLeadSocket(id, (event) => {
      const payload = event as { status?: string; type?: string; error?: string }
      if (!payload.status) return
      setJobStatus(`${payload.type || 'job'}: ${payload.status}${payload.error ? ` — ${payload.error}` : ''}`)
      if (payload.status === 'completed') {
        setScoring(false)
        void refresh()
      }
      if (payload.status === 'failed') {
        setScoring(false)
      }
    })
    return () => socket.close()
  }, [id])

  async function onScore() {
    if (!id) return
    setScoring(true)
    setJobStatus('score: queued')
    setError('')
    try {
      const job = await api.enqueueScore(id)
      setJobStatus(`score: ${job.status}`)
    } catch (err) {
      setScoring(false)
      setError(err instanceof Error ? err.message : 'Failed to enqueue score')
    }
  }

  async function onStatusChange(status: string) {
    if (!id) return
    try {
      setLead(await api.updateStatus(id, status))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update status')
    }
  }

  if (!lead) {
    return (
      <div className="app-shell">
        <p className="muted">{error || 'Loading lead…'}</p>
      </div>
    )
  }

  return (
    <div className="app-shell stack">
      <header className="topbar">
        <div className="brand">
          <strong>LeadScore</strong>
          <span>{lead.company_name}</span>
        </div>
        <Link className="btn ghost" to="/">
          All leads
        </Link>
      </header>

      {error ? <p className="error">{error}</p> : null}
      {jobStatus ? (
        <div className={`status-strip ${jobStatus.includes('failed') ? 'failed' : ''}`}>
          <strong>Live job</strong>
          <span>{jobStatus}</span>
        </div>
      ) : null}

      <div className="grid-2">
        <section className="panel panel-pad stack">
          <div>
            <h2>Lead profile</h2>
            <p className="muted">Firmographics and outcome status used by RAG scoring.</p>
          </div>
          <div className="stack">
            <div>
              <strong>{lead.company_name}</strong>
              <div className="muted">
                {lead.contact_name || 'No contact'} · {lead.email}
              </div>
            </div>
            <div className="muted">
              {lead.source} · {lead.industry || 'n/a'} · size {lead.company_size || 0}
            </div>
            <p>{lead.notes || 'No notes'}</p>
            <div className="field">
              <label>Status</label>
              <select value={lead.status} onChange={(e) => void onStatusChange(e.target.value)}>
                {statuses.map((status) => (
                  <option key={status} value={status}>
                    {status}
                  </option>
                ))}
              </select>
            </div>
            <button className="btn" type="button" disabled={scoring} onClick={() => void onScore()}>
              {scoring ? 'Scoring…' : 'Score now'}
            </button>
          </div>
        </section>

        <section className="panel panel-pad stack">
          <div>
            <h2>Latest score</h2>
            <p className="muted">Conversion probability with model reasoning.</p>
          </div>
          <div className="score-hero">
            <div className="muted">Conversion probability</div>
            <div className="value">{probability}</div>
            <div className="muted">{score?.model || 'No score yet'}</div>
            <p>{score?.reasoning || 'Run Score now to generate a RAG score.'}</p>
          </div>
        </section>
      </div>

      <section className="panel panel-pad stack">
        <div>
          <h2>Similar leads</h2>
          <p className="muted">Nearest neighbors from pgvector for RAG context.</p>
        </div>
        <div className="lead-list">
          {similar.map((item) => (
            <div key={item.id} className="lead-row">
              <div>
                <strong>{item.company_name}</strong>
                <div className="muted">
                  {item.source} · {item.status || 'n/a'} · sim {item.similarity.toFixed(2)}
                </div>
              </div>
              <span className="status-pill">{item.status || 'unknown'}</span>
            </div>
          ))}
          {similar.length === 0 ? <p className="muted">No similar leads yet.</p> : null}
        </div>
      </section>

      <section className="panel panel-pad stack">
        <div>
          <h2>Score history</h2>
          <p className="muted">Previous scoring runs for this lead.</p>
        </div>
        <div className="lead-list">
          {scores.map((item) => (
            <div key={item.id} className="lead-row">
              <div>
                <strong>{Math.round(item.conversion_probability * 100)}%</strong>
                <div className="muted">
                  {item.model} · {new Date(item.created_at).toLocaleString()}
                </div>
              </div>
            </div>
          ))}
          {scores.length === 0 ? <p className="muted">No score history yet.</p> : null}
        </div>
      </section>
    </div>
  )
}
