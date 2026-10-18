import type { ModelsResponse, Range } from '../api'
import { RequestsChart } from '../components/charts'
import { NavLink } from '../components/NavLink'
import { Cell, EmptyRow, PageStatus, Panel, PanelHead, Stats, Th } from '../components/ui'
import { formatCompact, formatCost, formatCount, formatLatency, rangeLabel } from '../format'
import { withSpend } from '../stats'
import { useApi } from '../useApi'

export function Models({ range }: { range: Range }) {
  const { data, error } = useApi<ModelsResponse>(`/models?range=${range}`)
  if (!data) return <PageStatus error={error} />

  const totals = data.totals
  const providers = new Set(data.models.map((row) => row.provider)).size
  const totalTokens = Math.max(1, data.models.reduce((sum, row) => sum + row.tokens, 0))
  const errorRate = totals.requests > 0 ? totals.errors / totals.requests : 0

  const stats = withSpend(
    [
      { label: 'Distinct models', value: formatCount(data.models.length), sub: `${formatCount(providers)} providers` },
      { label: 'Requests', value: formatCount(totals.requests), sub: `${formatCount(totals.errors)} upstream errors` },
      {
        label: 'Tokens',
        value: formatCompact(totals.inputTokens + totals.outputTokens),
        sub: `${formatCompact(totals.inputTokens)} in · ${formatCompact(totals.outputTokens)} out`,
      },
      { label: 'Error rate', value: (errorRate * 100).toFixed(2) + '%', sub: `${formatCount(totals.errors)} of ${formatCount(totals.requests)}` },
    ],
    totals,
  )

  return (
    <>
      <Stats items={stats} />

      <Panel>
        <PanelHead
          title="All models"
          subtitle="Requested model, resolved upstream"
          action={
            <NavLink page="providers" className="panel-action">
              Configure routes
            </NavLink>
          }
        />
        <div className="table">
          <div className="thead">
            <Th grow>Model</Th>
            <Th w={110}>Provider</Th>
            <Th w={96} right>
              Requests
            </Th>
            <Th w={104} right>
              Tokens
            </Th>
            <Th w={88} right>
              Cost
            </Th>
            <Th w={92} right>
              Latency
            </Th>
            <Th w={80} right>
              Errors
            </Th>
            <Th w={130} className="pad-16">
              Share
            </Th>
          </div>

          {data.models.length === 0 ? (
            <EmptyRow message="No usage recorded in this range." />
          ) : (
            data.models.map((row, index) => (
              <div className={index === data.models.length - 1 ? 'trow trow--last' : 'trow'} key={`${row.name}-${row.provider}`}>
                <Cell grow className="name-cell">
                  <span
                    className="row-dot"
                    style={{ background: `var(--color-chart-${(index % 5) + 1})` }}
                    aria-hidden="true"
                  />
                  <span className="mono cell-sm primary ellipsis">{row.name}</span>
                </Cell>
                <Cell w={110} className="cell-sm muted">
                  {row.provider}
                </Cell>
                <Cell w={96} right className="mono cell-sm muted">
                  {formatCount(row.requests)}
                </Cell>
                <Cell w={104} right className="mono cell-sm primary">
                  {formatCompact(row.tokens)}
                </Cell>
                <Cell w={88} right className="mono cell-sm muted">
                  {formatCost(row.costUSD)}
                </Cell>
                <Cell w={92} right className="mono cell-sm muted">
                  {formatLatency(row.avgLatencyMs)}
                </Cell>
                <Cell w={80} right className={row.errors > 0 ? 'mono cell-sm warning' : 'mono cell-sm muted'}>
                  {formatCount(row.errors)}
                </Cell>
                <Cell w={130} className="bar-cell pad-16">
                  <div className="track">
                    <div
                      className="track-fill"
                      style={{
                        width: `${(row.tokens / totalTokens) * 100}%`,
                        background: `var(--color-chart-${(index % 5) + 1})`,
                      }}
                    />
                  </div>
                </Cell>
              </div>
            ))
          )}

          {data.models.length > 0 ? (
            <div className="trow trow--total">
              <Cell grow className="cell-sm primary">
                Total
              </Cell>
              <Cell w={110} />
              <Cell w={96} right className="mono cell-sm primary">
                {formatCount(totals.requests)}
              </Cell>
              <Cell w={104} right className="mono cell-sm primary">
                {formatCompact(totals.inputTokens + totals.outputTokens)}
              </Cell>
              <Cell w={88} right className="mono cell-sm primary">
                {formatCost(totals.costUSD)}
              </Cell>
              <Cell w={92} right className="mono cell-sm primary">
                {formatLatency(totals.avgLatencyMs)}
              </Cell>
              <Cell w={80} right className="mono cell-sm primary">
                {formatCount(totals.errors)}
              </Cell>
              <Cell w={130} />
            </div>
          ) : null}
        </div>
      </Panel>

      <Panel>
        <div className="panel-head panel-head--top">
          <div>
            <h2 className="panel-title">Requests over time</h2>
            <p className="panel-sub">{rangeLabel(range)}</p>
          </div>
          <div className="panel-total">
            <span className="panel-total-value">{formatCount(totals.requests)}</span>
            <span className="panel-total-label">requests</span>
          </div>
        </div>
        <RequestsChart points={data.volume} range={range} />
      </Panel>
    </>
  )
}
