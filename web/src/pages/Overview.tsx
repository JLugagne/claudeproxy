import type { OverviewResponse, Range } from '../api'
import { Legend, VolumeChart } from '../components/charts'
import { NavLink } from '../components/NavLink'
import { BarRowList, Cell, EmptyRow, PageStatus, Panel, PanelHead, Pill, Stats, Th } from '../components/ui'
import { formatClock, formatCompact, formatCost, formatLatency, formatShare } from '../format'
import { trafficStats } from '../stats'
import { useApi } from '../useApi'

export function Overview({ range }: { range: Range }) {
  const { data, error } = useApi<OverviewResponse>(`/overview?range=${range}`)
  if (!data) return <PageStatus error={error} />

  const totals = data.totals
  const projectMax = Math.max(1, ...data.topProjects.map((row) => row.tokens))
  const modelTotal = data.topModels.reduce((sum, row) => sum + row.tokens, 0)

  return (
    <>
      <Stats items={trafficStats(totals)} />

      <Panel>
        <div className="panel-head panel-head--top">
          <div>
            <h2 className="panel-title">Token volume</h2>
            <div style={{ marginTop: 10 }}>
              <Legend />
            </div>
          </div>
          <div className="panel-total">
            <span className="panel-total-value">{formatCompact(totals.inputTokens + totals.outputTokens)}</span>
            <span className="panel-total-label">total tokens</span>
          </div>
        </div>
        <VolumeChart points={data.volume} range={range} />
      </Panel>

      <div className="grid-2">
        <Panel grow>
          <PanelHead
            title="Top projects"
            action={
              <NavLink page="projects" className="panel-action">
                View all
              </NavLink>
            }
          />
          {data.topProjects.length === 0 ? (
            <p className="panel-note">No usage recorded in this range.</p>
          ) : (
            <BarRowList
              rows={data.topProjects.map((row, index) => ({
                name: row.name,
                percent: (row.tokens / projectMax) * 100,
                value: formatCompact(row.tokens),
                color: index + 1,
              }))}
            />
          )}
        </Panel>

        <Panel grow>
          <PanelHead
            title="Models by tokens"
            action={
              <NavLink page="models" className="panel-action">
                View all
              </NavLink>
            }
          />
          {data.topModels.length === 0 ? (
            <p className="panel-note">No usage recorded in this range.</p>
          ) : (
            <BarRowList
              rows={data.topModels.map((row, index) => ({
                name: row.name,
                percent: modelTotal > 0 ? (row.tokens / modelTotal) * 100 : 0,
                value: formatShare(row.tokens, modelTotal),
                color: index + 1,
              }))}
            />
          )}
        </Panel>
      </div>

      <Panel>
        <PanelHead
          title="Recent requests"
          subtitle="Newest first"
          action={
            <NavLink page="requests" className="panel-action">
              View all
            </NavLink>
          }
        />
        <div className="table">
          <div className="thead">
            <Th w={100}>Time</Th>
            <Th grow>Project</Th>
            <Th grow>Model</Th>
            <Th w={96}>Provider</Th>
            <Th w={72} right>
              In
            </Th>
            <Th w={72} right>
              Out
            </Th>
            <Th w={80} right>
              Latency
            </Th>
            <Th w={80} right>
              TTFT
            </Th>
            <Th w={80} right>
              Cost
            </Th>
            <Th w={84} right>
              Status
            </Th>
          </div>

          {data.recent.length === 0 ? (
            <EmptyRow message="No request recorded in this range." />
          ) : (
            data.recent.map((row, index) => (
              <div
                className={index === data.recent.length - 1 ? 'trow trow--last' : 'trow'}
                key={`${row.time}-${index}`}
              >
                <Cell w={100} className="mono cell-sm muted">
                  {formatClock(row.time)}
                </Cell>
                <Cell grow className="cell-sm primary ellipsis">
                  {row.project}
                </Cell>
                <Cell grow className="mono cell-sm muted ellipsis">
                  {row.requestedModel}
                </Cell>
                <Cell w={96} className="cell-sm muted">
                  {row.provider}
                </Cell>
                <Cell w={72} right className="mono cell-sm muted">
                  {formatCompact(row.inputTokens)}
                </Cell>
                <Cell w={72} right className="mono cell-sm muted">
                  {formatCompact(row.outputTokens)}
                </Cell>
                <Cell w={80} right className="mono cell-sm muted">
                  {formatLatency(row.durationMs)}
                </Cell>
                <Cell w={80} right className="mono cell-sm muted">
                  {formatLatency(row.ttftMs)}
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
    </>
  )
}
