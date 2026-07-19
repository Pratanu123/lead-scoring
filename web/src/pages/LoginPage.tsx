import { useState, type FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { getApiKey, setApiKey } from '../api'

export default function LoginPage() {
  const navigate = useNavigate()
  const [key, setKey] = useState(getApiKey() || 'dev-lead-scoring-key')
  const [error, setError] = useState('')

  if (getApiKey()) return <Navigate to="/" replace />

  async function onSubmit(event: FormEvent) {
    event.preventDefault()
    setError('')
    const trimmed = key.trim()
    if (!trimmed) {
      setError('API key is required')
      return
    }
    setApiKey(trimmed)
    try {
      const response = await fetch('/v1/leads?limit=1', {
        headers: { Authorization: `Bearer ${trimmed}` },
      })
      if (!response.ok) {
        throw new Error('Invalid API key')
      }
      navigate('/')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    }
  }

  return (
    <div className="login-page">
      <form className="panel login-card" onSubmit={onSubmit}>
        <h1>LeadScore</h1>
        <p>Enter your local API key to open the scoring console.</p>
        <div className="stack">
          <div className="field">
            <label htmlFor="apiKey">API key</label>
            <input
              id="apiKey"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              autoComplete="off"
              placeholder="dev-lead-scoring-key"
            />
          </div>
          {error ? <div className="error">{error}</div> : null}
          <button className="btn" type="submit">
            Enter workspace
          </button>
        </div>
      </form>
    </div>
  )
}
