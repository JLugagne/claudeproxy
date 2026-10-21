export type Range = '24h' | '7d' | '30d'

export type Totals = {
  requests: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheCreationTokens: number
  cacheHitRate: number
  avgLatencyMs: number
  p95LatencyMs: number
  avgTtftMs: number
  p95TtftMs: number
  errors: number
  costUSD: number
  pricedRequests: number
}

export type Point = {
  start: string
  requests: number
  inputTokens: number
  outputTokens: number
}

export type Bar = {
  name: string
  requests: number
  tokens: number
}

export type RequestRow = {
  time: string
  project: string
  session: string
  provider: string
  requestedModel: string
  upstreamModel: string
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheCreationTokens: number
  status: number
  durationMs: number
  ttftMs: number
  stream: boolean
  costUSD: number
  hasCost: boolean
}

export type Project = {
  name: string
  sessions: number
  requests: number
  inputTokens: number
  outputTokens: number
  cacheHitRate: number
  avgLatencyMs: number
  avgTtftMs: number
  costUSD: number
}

export type Model = {
  name: string
  provider: string
  requests: number
  tokens: number
  avgLatencyMs: number
  avgTtftMs: number
  errors: number
  costUSD: number
}

export type Session = {
  session: string
  project: string
  started: string
  durationMs: number
  requests: number
  tokens: number
  topModel: string
  costUSD: number
}

export type Provider = {
  name: string
  baseUrl: string
  auth: string
  keyPresent: boolean
  requests: number
  tokens: number
  avgLatencyMs: number
  avgTtftMs: number
  errors: number
  costUSD: number
}

export type Route = {
  match: string
  regex: string
  provider: string
  model: string
}

export type OverviewResponse = {
  range: Range
  totals: Totals
  volume: Point[]
  topProjects: Bar[]
  topModels: Bar[]
  recent: RequestRow[]
}

export type ProjectsResponse = {
  range: Range
  totals: Totals
  volume: Point[]
  projects: Project[]
}

export type ModelsResponse = {
  range: Range
  totals: Totals
  volume: Point[]
  models: Model[]
}

export type RequestsResponse = {
  range: Range
  total: number
  rows: RequestRow[]
}

export type SessionsResponse = {
  range: Range
  totals: Totals
  sessions: Session[]
}

export type ProvidersResponse = {
  range: Range
  metricsEnabled: boolean
  defaultProvider: string
  providers: Provider[]
  routes: Route[]
}

export type QuotaWindow = {
  utilization: number
  resetsAt: string | null
  status: string
  present: boolean
}

export type QuotaCurrent = {
  time: string
  fiveHour: QuotaWindow
  sevenDay: QuotaWindow
  claim: string
}

export type QuotaPoint = {
  time: string
  fiveHour: number | null
  sevenDay: number | null
}

export type QuotaShare = {
  name: string
  project?: string
  fiveHour: number
  sevenDay: number
}

export type QuotaResponse = {
  range: Range
  current: QuotaCurrent | null
  series: QuotaPoint[]
  projects: QuotaShare[]
  sessions: QuotaShare[]
}

export async function apiGet<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`/dashboard/api${path}`, {
    signal,
    headers: { Accept: 'application/json' },
  })
  if (!response.ok) {
    let message = `request failed (${response.status})`
    try {
      const body = (await response.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // keep the status message
    }
    throw new Error(message)
  }
  return (await response.json()) as T
}
