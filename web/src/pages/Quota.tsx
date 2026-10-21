import type { QuotaResponse, QuotaShare, Range } from '../api'
import { QuotaChart, QuotaLegend } from '../components/charts'
import { QuotaMeter } from '../components/QuotaMeters'
import { Cell, EmptyRow, NoteBar, PageStatus, Panel, PanelHead, Pill, Th } from '../components/ui'
import { rangeLabel } from '../format'
import { useApi } from '../useApi'

function formatPoints(ratio: number): string {
  if (ratio <= 0) return '—'
  return `${(ratio * 100).toFixed(1)} pts`
}

function claimLabel(claim: string): string {
  switch (claim) {
    case 'five_hour':
      return '5-hour window'
    case 'seven_day':
      return 'Weekly window'
    default:
      return claim || '—'
  }
}

function SharesTable({ title, subtitle, rows, withProject }: { title: string; subtitle: string; rows: QuotaShare[]; withProject?: boolean }) {
  return (
    <Panel>
      <PanelHead title={title} subtitle={subtitle} />
      <div className="table">
        <div className="thead">
          <Th grow>{withProject ? 'Session' : 'Project'}</Th>
          {withProject ? <Th grow>Project</Th> : null}
          <Th w={96} right>
            5h
          </Th>
          <Th w={96} right>
            Weekly
          </Th>
        </div>
        {rows.length === 0 ? (
          <EmptyRow message="No quota movement recorded in this range." />
        ) : (
          rows.map((row, index) => (
            <div className={index === rows.length - 1 ? 'trow trow--last' : 'trow'} key={`${row.name}-${row.project ?? ''}`}>
              <Cell grow className={withProject ? 'mono cell-sm primary ellipsis' : 'cell-sm primary ellipsis'}>
                {row.name}
              </Cell>
              {withProject ? (
                <Cell grow className="cell-sm muted ellipsis">
                  {row.project}
                </Cell>
              ) : null}
              <Cell w={96} right className="mono cell-sm muted">
                {formatPoints(row.fiveHour)}
              </Cell>
              <Cell w={96} right className="mono cell-sm primary">
                {formatPoints(row.sevenDay)}
              </Cell>
            </div>
          ))
        )}
      </div>
    </Panel>
  )
}

export function Quota({ range }: { range: Range }) {
  const { data, error } = useApi<QuotaResponse>(`/quota?range=${range}`)
  if (!data) return <PageStatus error={error} />

  if (!data.current) {
    return (
      <Panel>
        <div className="message">
          <h2 className="panel-title">No quota data in this range</h2>
          <p className="panel-sub">
            Quota comes from the anthropic-ratelimit-unified-* headers of Anthropic responses. It appears after the first
            request routed to Anthropic with a Claude subscription.
          </p>
        </div>
      </Panel>
    )
  }

  const current = data.current

  return (
    <>
      <div className="meter-grid">
        <Panel>
          <QuotaMeter label="5-hour window" window={current.fiveHour} />
        </Panel>
        <Panel>
          <QuotaMeter label="Weekly window" window={current.sevenDay} />
        </Panel>
        <Panel>
          <div className="meter">
            <div className="meter-head">
              <span className="meter-label">Limiting window</span>
              <Pill tone={current.fiveHour.status === 'allowed' && current.sevenDay.status === 'allowed' ? 'success' : 'warning'}>
                {current.fiveHour.status === 'allowed' && current.sevenDay.status === 'allowed' ? 'allowed' : 'limited'}
              </Pill>
            </div>
            <span className="meter-value meter-value--lg">{claimLabel(current.claim)}</span>
            <span className="meter-sub">the one that runs out first</span>
          </div>
        </Panel>
      </div>

      <Panel>
        <div className="panel-head panel-head--top">
          <div>
            <h2 className="panel-title">Utilization</h2>
            <div style={{ marginTop: 10 }}>
              <QuotaLegend />
            </div>
          </div>
          <span className="panel-sub">{rangeLabel(range)}</span>
        </div>
        <QuotaChart points={data.series} range={range} />
      </Panel>

      <div className="grid-2">
        <SharesTable title="Quota by project" subtitle="Points of quota consumed in the range" rows={data.projects} />
        <SharesTable title="Quota by session" subtitle="Points of quota consumed in the range" rows={data.sessions} withProject />
      </div>

      <NoteBar label="How it is measured">
        Each rise of a window&apos;s utilization is charged to the request that observed it, so concurrent sessions share
        the credit approximately. Snapshots are kept every 30 minutes per session. A drop is a window reset and is not
        charged.
      </NoteBar>
    </>
  )
}
