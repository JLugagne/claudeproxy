import type { Range, SessionsResponse } from '../api'
import { Cell, EmptyRow, PageStatus, Panel, PanelHead, Stats, Th } from '../components/ui'
import {
  formatCompact,
  formatCost,
  formatCount,
  formatDuration,
  formatTime,
  rangeLabel,
} from '../format'
import { withSpend } from '../stats'
import { useApi } from '../useApi'
import { downloadCSV } from '../util'

function median(values: number[]): number {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  return sorted[Math.floor(sorted.length / 2)]
}

function p95(values: number[]): number {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  return sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * 0.95))]
}

export function Sessions({ range }: { range: Range }) {
  const { data, error } = useApi<SessionsResponse>(`/sessions?range=${range}&limit=200`)
  if (!data) return <PageStatus error={error} />

  const totals = data.totals
  const sessions = data.sessions
  const tokenValues = sessions.map((row) => row.tokens)
  const durationValues = sessions.map((row) => row.durationMs)
  const average = (values: number[]) =>
    values.length === 0 ? 0 : values.reduce((sum, value) => sum + value, 0) / values.length

  const stats = withSpend(
    [
      { label: 'Sessions', value: formatCount(sessions.length), sub: `${formatCount(totals.requests)} requests` },
      {
        label: 'Avg tokens / session',
        value: formatCompact(average(tokenValues)),
        sub: `median ${formatCompact(median(tokenValues))}`,
      },
      {
        label: 'Avg duration',
        value: formatDuration(average(durationValues)),
        sub: `p95 ${formatDuration(p95(durationValues))}`,
      },
    ],
    totals,
  )

  const exportCSV = () =>
    downloadCSV(
      `claudeproxy-sessions-${range}.csv`,
      ['session', 'project', 'started', 'duration_ms', 'requests', 'tokens', 'top_model', 'cost_usd'],
      sessions.map((row) => [
        row.session,
        row.project,
        row.started,
        row.durationMs,
        row.requests,
        row.tokens,
        row.topModel,
        row.costUSD.toFixed(6),
      ]),
    )

  return (
    <>
      <Stats items={stats} />

      <Panel>
        <PanelHead
          title="Recent sessions"
          subtitle={`${rangeLabel(range)} — one conversation per Claude Code run`}
          action={
            <button type="button" className="panel-action" onClick={exportCSV}>
              Export CSV
            </button>
          }
        />
        <div className="table">
          <div className="thead">
            <Th w={110}>Session</Th>
            <Th grow>Project</Th>
            <Th w={70}>Started</Th>
            <Th w={84} right>
              Duration
            </Th>
            <Th w={88} right>
              Requests
            </Th>
            <Th w={96} right>
              Tokens
            </Th>
            <Th w={80} right>
              Cost
            </Th>
            <Th grow className="pad-left">
              Top model
            </Th>
          </div>

          {sessions.length === 0 ? (
            <EmptyRow message="No session recorded in this range." />
          ) : (
            sessions.map((row, index) => (
              <div className={index === sessions.length - 1 ? 'trow trow--last' : 'trow'} key={`${row.session}-${row.project}`}>
                <Cell w={110} className="mono cell-sm primary ellipsis">
                  {row.session}
                </Cell>
                <Cell grow className="cell-sm primary ellipsis">
                  {row.project}
                </Cell>
                <Cell w={70} className="mono cell-sm muted">
                  {formatTime(row.started)}
                </Cell>
                <Cell w={84} right className="mono cell-sm muted">
                  {formatDuration(row.durationMs)}
                </Cell>
                <Cell w={88} right className="mono cell-sm muted">
                  {formatCount(row.requests)}
                </Cell>
                <Cell w={96} right className="mono cell-sm primary">
                  {formatCompact(row.tokens)}
                </Cell>
                <Cell w={80} right className="mono cell-sm muted">
                  {formatCost(row.costUSD)}
                </Cell>
                <Cell grow className="mono cell-sm muted ellipsis pad-left">
                  {row.topModel || '—'}
                </Cell>
              </div>
            ))
          )}
        </div>
      </Panel>
    </>
  )
}
