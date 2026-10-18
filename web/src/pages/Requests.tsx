import { useEffect, useState } from 'react'
import type { Range, RequestsResponse } from '../api'
import { Cell, EmptyRow, PageStatus, Panel, Pill, Th } from '../components/ui'
import { formatClock, formatCompact, formatCost, formatCount, formatLatency } from '../format'
import { useApi } from '../useApi'
import { downloadCSV } from '../util'

export function Requests({ range }: { range: Range }) {
  const [query, setQuery] = useState('')
  const [debounced, setDebounced] = useState('')
  const [status, setStatus] = useState('all')
  const [streamingOnly, setStreamingOnly] = useState(false)

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(query), 300)
    return () => window.clearTimeout(timer)
  }, [query])

  const params = new URLSearchParams({ range, limit: '200' })
  if (debounced.trim()) params.set('q', debounced.trim())
  if (status !== 'all') params.set('status', status)
  if (streamingOnly) params.set('stream', 'true')

  const { data, error } = useApi<RequestsResponse>(`/requests?${params.toString()}`)

  const rows = data?.rows ?? []
  const exportCSV = () => {
    if (!data) return
    downloadCSV(
      `claudeproxy-requests-${range}.csv`,
      [
        'time',
        'project',
        'session',
        'provider',
        'requested_model',
        'upstream_model',
        'input_tokens',
        'output_tokens',
        'cache_read_tokens',
        'cache_creation_tokens',
        'latency_ms',
        'cost_usd',
        'status',
        'stream',
      ],
      data.rows.map((row) => [
        row.time,
        row.project,
        row.session,
        row.provider,
        row.requestedModel,
        row.upstreamModel,
        row.inputTokens,
        row.outputTokens,
        row.cacheReadTokens,
        row.cacheCreationTokens,
        row.durationMs,
        row.costUSD.toFixed(6),
        row.status,
        row.stream ? 'true' : 'false',
      ]),
    )
  }

  return (
    <>
      <div className="filters">
        <input
          className="input"
          style={{ width: 300 }}
          placeholder="Filter by project, model or session"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          aria-label="Filter requests"
        />
        <select
          className="select"
          style={{ width: 170 }}
          value={status}
          onChange={(event) => setStatus(event.target.value)}
          aria-label="Filter by status"
        >
          <option value="all">All statuses</option>
          <option value="200">200 OK</option>
          <option value="429">429 rate limited</option>
          <option value="500">500 server error</option>
        </select>
        <button
          type="button"
          className="chip-btn"
          data-active={streamingOnly}
          aria-pressed={streamingOnly}
          onClick={() => setStreamingOnly((value) => !value)}
        >
          Streaming only
        </button>
        <span className="row-count">{data ? `${formatCount(data.total)} requests` : '…'}</span>
      </div>

      {!data ? (
        <PageStatus error={error} />
      ) : (
        <Panel>
          <div className="panel-head">
            <div>
              <h2 className="panel-title">Request log</h2>
              <p className="panel-sub">Newest first — showing {formatCount(rows.length)} of {formatCount(data.total)}</p>
            </div>
            <button type="button" className="panel-action" onClick={exportCSV}>
              Export CSV
            </button>
          </div>

          <div className="table">
            <div className="thead">
              <Th w={90}>Time</Th>
              <Th grow>Project</Th>
              <Th w={100}>Session</Th>
              <Th grow>Model</Th>
              <Th w={90}>Provider</Th>
              <Th w={68} right>
                In
              </Th>
              <Th w={68} right>
                Out
              </Th>
              <Th w={70} right>
                Cache
              </Th>
              <Th w={76} right>
                Latency
              </Th>
              <Th w={80} right>
                Cost
              </Th>
              <Th w={84} right>
                Status
              </Th>
            </div>

            {rows.length === 0 ? (
              <EmptyRow message="No request matches these filters." />
            ) : (
              rows.map((row, index) => (
                <div className={index === rows.length - 1 ? 'trow trow--last' : 'trow'} key={`${row.time}-${index}`}>
                  <Cell w={90} className="mono cell-sm muted">
                    {formatClock(row.time)}
                  </Cell>
                  <Cell grow className="cell-sm primary ellipsis">
                    {row.project}
                  </Cell>
                  <Cell w={100} className="mono cell-sm faint ellipsis">
                    {row.session}
                  </Cell>
                  <Cell grow className="mono cell-sm muted ellipsis">
                    {row.requestedModel}
                  </Cell>
                  <Cell w={90} className="cell-sm muted">
                    {row.provider}
                  </Cell>
                  <Cell w={68} right className="mono cell-sm muted">
                    {formatCompact(row.inputTokens)}
                  </Cell>
                  <Cell w={68} right className="mono cell-sm muted">
                    {formatCompact(row.outputTokens)}
                  </Cell>
                  <Cell w={70} right className="mono cell-sm muted">
                    {formatCompact(row.cacheReadTokens + row.cacheCreationTokens)}
                  </Cell>
                  <Cell w={76} right className="mono cell-sm muted">
                    {formatLatency(row.durationMs)}
                  </Cell>
                  <Cell w={80} right className="mono cell-sm muted">
                    {formatCost(row.costUSD)}
                  </Cell>
                  <Cell w={84} className="pill-cell">
                    <Pill tone={row.status < 400 ? 'success' : 'warning'} mono>
                      {row.status}
                    </Pill>
                  </Cell>
                </div>
              ))
            )}
          </div>
        </Panel>
      )}
    </>
  )
}
