import type { ProjectsResponse, Range } from '../api'
import { RequestsChart } from '../components/charts'
import { Cell, EmptyRow, PageStatus, Panel, PanelHead, Stats, Th } from '../components/ui'
import { formatCompact, formatCost, formatCount, formatLatency, formatPercent, rangeLabel } from '../format'
import { trafficStats } from '../stats'
import { useApi } from '../useApi'
import { downloadCSV } from '../util'

export function Projects({ range }: { range: Range }) {
  const { data, error } = useApi<ProjectsResponse>(`/projects?range=${range}`)
  if (!data) return <PageStatus error={error} />

  const totals = data.totals
  const maxTokens = Math.max(1, ...data.projects.map((row) => row.inputTokens + row.outputTokens))

  const exportCSV = () =>
    downloadCSV(
      `claudeproxy-projects-${range}.csv`,
      ['project', 'sessions', 'requests', 'input_tokens', 'output_tokens', 'cache_hit_rate', 'avg_latency_ms', 'cost_usd'],
      data.projects.map((row) => [
        row.name,
        row.sessions,
        row.requests,
        row.inputTokens,
        row.outputTokens,
        row.cacheHitRate.toFixed(4),
        Math.round(row.avgLatencyMs),
        row.costUSD.toFixed(6),
      ]),
    )

  return (
    <>
      <Stats items={trafficStats(totals)} />

      <Panel>
        <PanelHead
          title="All projects"
          subtitle={`${formatCount(data.projects.length)} projects in range`}
          action={
            <button type="button" className="panel-action" onClick={exportCSV}>
              Export CSV
            </button>
          }
        />
        <div className="table">
          <div className="thead">
            <Th grow>Project</Th>
            <Th w={90}>Sessions</Th>
            <Th w={96} right>
              Requests
            </Th>
            <Th w={104} right>
              Tokens
            </Th>
            <Th w={80} right>
              Cache
            </Th>
            <Th w={92} right>
              Latency
            </Th>
            <Th w={88} right>
              Cost
            </Th>
            <Th w={130} className="pad-16">
              Share
            </Th>
          </div>

          {data.projects.length === 0 ? (
            <EmptyRow message="No usage recorded in this range." />
          ) : (
            data.projects.map((row, index) => {
              const tokens = row.inputTokens + row.outputTokens
              return (
                <div className={index === data.projects.length - 1 ? 'trow trow--last' : 'trow'} key={row.name}>
                  <Cell grow className="name-cell">
                    <span
                      className="row-dot"
                      style={{ background: `var(--color-chart-${(index % 5) + 1})` }}
                      aria-hidden="true"
                    />
                    <span className="cell-sm primary ellipsis">{row.name}</span>
                  </Cell>
                  <Cell w={90} className="mono cell-sm muted">
                    {formatCount(row.sessions)}
                  </Cell>
                  <Cell w={96} right className="mono cell-sm muted">
                    {formatCount(row.requests)}
                  </Cell>
                  <Cell w={104} right className="mono cell-sm primary">
                    {formatCompact(tokens)}
                  </Cell>
                  <Cell w={80} right className="mono cell-sm muted">
                    {formatPercent(row.cacheHitRate)}
                  </Cell>
                  <Cell w={92} right className="mono cell-sm muted">
                    {formatLatency(row.avgLatencyMs)}
                  </Cell>
                  <Cell w={88} right className="mono cell-sm muted">
                    {formatCost(row.costUSD)}
                  </Cell>
                  <Cell w={130} className="bar-cell pad-16">
                    <div className="track">
                      <div
                        className="track-fill"
                        style={{
                          width: `${(tokens / maxTokens) * 100}%`,
                          background: `var(--color-chart-${(index % 5) + 1})`,
                        }}
                      />
                    </div>
                  </Cell>
                </div>
              )
            })
          )}

          {data.projects.length > 0 ? (
            <div className="trow trow--total">
              <Cell grow className="cell-sm primary">
                Total
              </Cell>
              <Cell w={90} className="mono cell-sm primary">
                {formatCount(data.projects.reduce((sum, row) => sum + row.sessions, 0))}
              </Cell>
              <Cell w={96} right className="mono cell-sm primary">
                {formatCount(totals.requests)}
              </Cell>
              <Cell w={104} right className="mono cell-sm primary">
                {formatCompact(totals.inputTokens + totals.outputTokens)}
              </Cell>
              <Cell w={80} right className="mono cell-sm primary">
                {formatPercent(totals.cacheHitRate)}
              </Cell>
              <Cell w={92} right className="mono cell-sm primary">
                {formatLatency(totals.avgLatencyMs)}
              </Cell>
              <Cell w={88} right className="mono cell-sm primary">
                {formatCost(totals.costUSD)}
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
