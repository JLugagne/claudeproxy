import type { ProvidersResponse, Range } from '../api'
import { Cell, EmptyRow, NoteBar, PageStatus, Panel, PanelHead, Pill, Stats, Th } from '../components/ui'
import { formatCompact, formatCost, formatCount, formatLatency, formatShare } from '../format'
import { useApi } from '../useApi'

export function Providers({ range }: { range: Range }) {
  const { data, error } = useApi<ProvidersResponse>(`/providers?range=${range}`)
  if (!data) return <PageStatus error={error} />

  const requests = data.providers.reduce((sum, provider) => sum + provider.requests, 0)
  const tokens = data.providers.reduce((sum, provider) => sum + provider.tokens, 0)
  const errors = data.providers.reduce((sum, provider) => sum + provider.errors, 0)
  const cost = data.providers.reduce((sum, provider) => sum + provider.costUSD, 0)
  const active = data.providers.filter((provider) => provider.requests > 0).length

  const stats = [
    { label: 'Providers', value: formatCount(data.providers.length), sub: `${formatCount(active)} with usage` },
    { label: 'Upstream requests', value: formatCount(requests), sub: `${formatCount(errors)} upstream errors` },
    { label: 'Tokens', value: formatCompact(tokens), sub: 'input + output' },
    {
      label: 'Spend',
      value: cost > 0 ? formatCost(cost) : '—',
      sub: cost > 0 ? 'reported by providers' : 'no provider reported a price',
    },
  ]

  return (
    <>
      <Stats items={stats} />

      <div className="cards">
        {data.providers.map((provider, index) => (
          <div className="card" key={provider.name}>
            <div className="card-row">
              <span className="card-name">{provider.name}</span>
              <Pill tone={provider.errors > 0 ? 'warning' : 'success'}>
                {provider.errors > 0 ? `${formatCount(provider.errors)} errors` : 'healthy'}
              </Pill>
            </div>
            <div className="divider" />
            <div className="field">
              <span className="field-label">Base URL</span>
              <span className="field-value primary">{provider.baseUrl}</span>
            </div>
            <div className="field">
              <span className="field-label">Auth</span>
              <span className="field-value primary">
                {provider.auth}
                {provider.auth !== 'passthrough' ? (provider.keyPresent ? ' — key present' : ' — key missing') : ''}
              </span>
            </div>
            <div className="metrics">
              <div className="metric">
                <span className="metric-value">{formatCount(provider.requests)}</span>
                <span className="metric-label">requests</span>
              </div>
              <div className="metric">
                <span className="metric-value">{formatCompact(provider.tokens)}</span>
                <span className="metric-label">tokens</span>
              </div>
              <div className="metric">
                <span className="metric-value">{formatLatency(provider.avgLatencyMs)}</span>
                <span className="metric-label">avg latency</span>
              </div>
              <div className="metric">
                <span className="metric-value">{formatLatency(provider.avgTtftMs)}</span>
                <span className="metric-label">avg TTFT</span>
              </div>
              <div className="metric">
                <span className="metric-value">{formatCost(provider.costUSD)}</span>
                <span className="metric-label">spend</span>
              </div>
            </div>
            <div className="share">
              <div className="share-row">
                <span>share of requests</span>
                <span>{formatShare(provider.requests, requests)}</span>
              </div>
              <div className="track">
                <div
                  className="track-fill"
                  style={{
                    width: `${requests > 0 ? (provider.requests / requests) * 100 : 0}%`,
                    background: `var(--color-chart-${(index % 5) + 1})`,
                  }}
                />
              </div>
            </div>
          </div>
        ))}
      </div>

      <Panel>
        <PanelHead title="Routes" subtitle="First match wins" />
        <div className="table">
          <div className="thead">
            <Th w={28}>#</Th>
            <Th grow>Match</Th>
            <Th w={70}>Type</Th>
            <Th grow>Provider</Th>
            <Th grow>Upstream model</Th>
            <Th w={90} right>
              Status
            </Th>
          </div>

          {data.routes.length === 0 ? (
            <EmptyRow message="No route configured." />
          ) : (
            data.routes.map((route, index) => (
              <div className={index === data.routes.length - 1 ? 'trow trow--last' : 'trow'} key={`${route.match}-${route.regex}-${index}`}>
                <Cell w={28} className="mono cell-sm faint">
                  {index + 1}
                </Cell>
                <Cell grow className="mono cell-sm primary ellipsis">
                  {route.match || route.regex}
                </Cell>
                <Cell w={70} className="cell-sm muted">
                  {route.match ? 'glob' : 'regex'}
                </Cell>
                <Cell grow className="mono cell-sm primary">
                  {route.provider}
                </Cell>
                <Cell grow className="mono cell-sm faint ellipsis">
                  {route.model || '—'}
                </Cell>
                <Cell w={90} className="pill-cell">
                  <Pill tone="success">enabled</Pill>
                </Cell>
              </div>
            ))
          )}
        </div>
        <p className="panel-note">
          Route-level counts are not recorded: usage is attributed to the provider. Unmatched models fall back to the
          default provider — {data.defaultProvider || 'none'}.
        </p>
      </Panel>

      <NoteBar label="config">
        Providers, routes and credentials are read from configs/config.yaml — this console is read-only, there is no
        settings page.
      </NoteBar>
    </>
  )
}
