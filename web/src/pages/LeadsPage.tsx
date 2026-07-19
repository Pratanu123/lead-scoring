import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, clearApiKey, type CreateLeadInput, type Lead } from '../api'

const emptyForm: CreateLeadInput = {
  company_name: '',
  contact_name: '',
  email: '',
  phone: '',
  source: 'website',
  industry: '',
  company_size: 100,
  annual_revenue: 1000000,
  notes: '',
}

export default function LeadsPage() {
  const navigate = useNavigate()
  const [leads, setLeads] = useState<Lead[]>([])
  const [form, setForm] = useState<CreateLeadInput>(emptyForm)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  async function load() {
    setLoading(true)
    setError('')
    try {
      setLeads(await api.listLeads(50))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load leads')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  async function onCreate(event: FormEvent) {
    event.preventDefault()
    setSaving(true)
    setError('')
    try {
      const lead = await api.createLead({
        ...form,
        company_size: Number(form.company_size) || 0,
        annual_revenue: Number(form.annual_revenue) || 0,
      })
      setForm(emptyForm)
      await load()
      navigate(`/leads/${lead.id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create lead')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="app-shell stack">
      <header className="topbar">
        <div className="brand">
          <strong>LeadScore</strong>
          <span>Local scoring console</span>
        </div>
        <button
          className="btn secondary"
          type="button"
          onClick={() => {
            clearApiKey()
            navigate('/login')
          }}
        >
          Sign out
        </button>
      </header>

      <div className="grid-2">
        <section className="panel panel-pad stack">
          <div>
            <h2>Leads</h2>
            <p className="muted">Recent CRM leads ready for embedding and conversion scoring.</p>
          </div>
          {loading ? <p className="muted">Loading…</p> : null}
          {error ? <p className="error">{error}</p> : null}
          <div className="lead-list">
            {leads.map((lead) => (
              <Link key={lead.id} className="lead-row" to={`/leads/${lead.id}`}>
                <div>
                  <strong>{lead.company_name}</strong>
                  <div className="muted">
                    {lead.email} · {lead.source}
                  </div>
                </div>
                <span className="status-pill">{lead.status}</span>
              </Link>
            ))}
            {!loading && leads.length === 0 ? <p className="muted">No leads yet. Create one to begin.</p> : null}
          </div>
        </section>

        <section className="panel panel-pad stack">
          <div>
            <h2>Create lead</h2>
            <p className="muted">Adds the lead and queues embedding in the background.</p>
          </div>
          <form className="stack" onSubmit={onCreate}>
            <div className="form-grid">
              <div className="field">
                <label>Company</label>
                <input
                  required
                  value={form.company_name}
                  onChange={(e) => setForm({ ...form, company_name: e.target.value })}
                />
              </div>
              <div className="field">
                <label>Contact</label>
                <input
                  value={form.contact_name}
                  onChange={(e) => setForm({ ...form, contact_name: e.target.value })}
                />
              </div>
              <div className="field">
                <label>Email</label>
                <input
                  required
                  type="email"
                  value={form.email}
                  onChange={(e) => setForm({ ...form, email: e.target.value })}
                />
              </div>
              <div className="field">
                <label>Source</label>
                <input
                  required
                  value={form.source}
                  onChange={(e) => setForm({ ...form, source: e.target.value })}
                />
              </div>
              <div className="field">
                <label>Industry</label>
                <input
                  value={form.industry}
                  onChange={(e) => setForm({ ...form, industry: e.target.value })}
                />
              </div>
              <div className="field">
                <label>Company size</label>
                <input
                  type="number"
                  value={form.company_size}
                  onChange={(e) => setForm({ ...form, company_size: Number(e.target.value) })}
                />
              </div>
            </div>
            <div className="field">
              <label>Notes</label>
              <textarea
                value={form.notes}
                onChange={(e) => setForm({ ...form, notes: e.target.value })}
              />
            </div>
            <button className="btn" disabled={saving} type="submit">
              {saving ? 'Creating…' : 'Create lead'}
            </button>
          </form>
        </section>
      </div>
    </div>
  )
}
