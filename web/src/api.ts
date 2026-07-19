export type Lead = {
  id: string
  company_name: string
  contact_name?: string
  email: string
  phone?: string
  source: string
  industry?: string
  company_size?: number
  annual_revenue?: number
  notes?: string
  status: string
  created_at: string
  updated_at: string
}

export type LeadScore = {
  id: string
  lead_id: string
  conversion_probability: number
  reasoning: string
  model: string
  created_at: string
}

export type SimilarLead = {
  id: string
  company_name: string
  source: string
  industry?: string
  company_size?: number
  annual_revenue?: number
  notes?: string
  status?: string
  similarity: number
}

export type Job = {
  id: string
  type: string
  lead_id: string
  status: string
  error?: string
  result?: unknown
  created_at: string
  updated_at: string
}

export type CreateLeadInput = {
  company_name: string
  contact_name?: string
  email: string
  phone?: string
  source: string
  industry?: string
  company_size?: number
  annual_revenue?: number
  notes?: string
}

const KEY = 'lead_scoring_api_key'

export function getApiKey(): string {
  return sessionStorage.getItem(KEY) || ''
}

export function setApiKey(key: string) {
  sessionStorage.setItem(KEY, key)
}

export function clearApiKey() {
  sessionStorage.removeItem(KEY)
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${getApiKey()}`)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(path, { ...init, headers })
  if (!response.ok) {
    let message = `Request failed (${response.status})`
    try {
      const payload = await response.json()
      if (payload?.error) message = payload.error
    } catch {
      // ignore
    }
    throw new Error(message)
  }
  if (response.status === 204) return undefined as T
  return response.json()
}

export const api = {
  listLeads: (limit = 20) =>
    request<{ items?: Lead[] } | Lead[]>(`/v1/leads?limit=${limit}`).then((data) =>
      Array.isArray(data) ? data : data.items || [],
    ),
  createLead: (input: CreateLeadInput) =>
    request<Lead>('/v1/leads', { method: 'POST', body: JSON.stringify(input) }),
  getLead: (id: string) => request<Lead>(`/v1/leads/${id}`),
  updateStatus: (id: string, status: string) =>
    request<Lead>(`/v1/leads/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ status }),
    }),
  enqueueScore: (id: string) =>
    request<{ job_id: string; status: string }>(`/v1/leads/${id}/score`, { method: 'POST' }),
  latestScore: (id: string) => request<LeadScore>(`/v1/leads/${id}/score`),
  listScores: (id: string) =>
    request<{ items: LeadScore[] }>(`/v1/leads/${id}/scores?limit=20`).then((d) => d.items || []),
  similar: (id: string) =>
    request<{ items: SimilarLead[] }>(`/v1/leads/${id}/similar?limit=5`).then((d) => d.items || []),
  getJob: (id: string) => request<Job>(`/v1/jobs/${id}`),
}

export function connectLeadSocket(leadId: string, onEvent: (event: unknown) => void) {
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${protocol}://${window.location.host}/v1/ws?lead_id=${encodeURIComponent(leadId)}&token=${encodeURIComponent(getApiKey())}`
  const socket = new WebSocket(url)
  socket.onmessage = (message) => {
    try {
      onEvent(JSON.parse(message.data))
    } catch {
      // ignore
    }
  }
  return socket
}
