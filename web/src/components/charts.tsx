import type { Point, QuotaPoint, Range } from '../api'
import { formatBucketLabel, formatCompact, niceMax } from '../format'

export function VolumeChart({ points, range }: { points: Point[]; range: Range }) {
  const peak = Math.max(0, ...points.map((point) => point.inputTokens + point.outputTokens))
  const max = niceMax(peak)
  const ticks = [max, max * 0.75, max * 0.5, max * 0.25, 0]

  const labels =
    points.length === 0
      ? []
      : [0, Math.floor(points.length / 4), Math.floor(points.length / 2), Math.floor((points.length * 3) / 4), points.length - 1]

  return (
    <div className="chart">
      <div className="yaxis" aria-hidden="true">
        {ticks.map((tick) => (
          <span key={tick}>{formatCompact(tick)}</span>
        ))}
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="plot">
          <div className="grid-line" style={{ top: '25%' }} />
          <div className="grid-line" style={{ top: '50%' }} />
          <div className="grid-line" style={{ top: '75%' }} />
          {points.map((point) => (
            <div
              className="bar-col"
              key={point.start}
              title={`${formatBucketLabel(point.start, range)} · ${formatCompact(point.inputTokens)} in · ${formatCompact(point.outputTokens)} out`}
            >
              <div className="bar-output" style={{ height: `${(point.outputTokens / max) * 100}%` }} />
              <div className="bar-input" style={{ height: `${(point.inputTokens / max) * 100}%` }} />
            </div>
          ))}
        </div>
        <div className="xaxis">
          {labels.map((index) => (
            <span key={index} style={{ left: `${((index + 0.5) / points.length) * 100}%` }}>
              {formatBucketLabel(points[index].start, range)}
            </span>
          ))}
        </div>
      </div>
    </div>
  )
}

/** A single-series bar chart of request counts over the range. */
export function RequestsChart({ points, range }: { points: Point[]; range: Range }) {
  const max = niceMax(Math.max(0, ...points.map((point) => point.requests)))
  const ticks = [max, max * 0.75, max * 0.5, max * 0.25, 0]
  const labels =
    points.length === 0
      ? []
      : [0, Math.floor(points.length / 4), Math.floor(points.length / 2), Math.floor((points.length * 3) / 4), points.length - 1]

  return (
    <div className="chart">
      <div className="yaxis" aria-hidden="true">
        {ticks.map((tick) => (
          <span key={tick}>{formatCompact(tick)}</span>
        ))}
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="plot">
          <div className="grid-line" style={{ top: '25%' }} />
          <div className="grid-line" style={{ top: '50%' }} />
          <div className="grid-line" style={{ top: '75%' }} />
          {points.map((point) => (
            <div
              className="bar-col"
              key={point.start}
              title={`${formatBucketLabel(point.start, range)} · ${point.requests} requests`}
            >
              <div className="bar-input" style={{ height: `${(point.requests / max) * 100}%`, borderRadius: '4px 4px 0 0' }} />
            </div>
          ))}
        </div>
        <div className="xaxis">
          {labels.map((index) => (
            <span key={index} style={{ left: `${((index + 0.5) / points.length) * 100}%` }}>
              {formatBucketLabel(points[index].start, range)}
            </span>
          ))}
        </div>
      </div>
    </div>
  )
}

export function Legend() {
  return (
    <div className="legend">
      <span className="legend-item">
        <span className="legend-swatch" style={{ background: 'var(--color-chart-1)' }} />
        Input
      </span>
      <span className="legend-item">
        <span className="legend-swatch" style={{ background: 'var(--color-chart-2)' }} />
        Output
      </span>
    </div>
  )
}

const QUOTA_SERIES = [
  { key: 'fiveHour', label: '5-hour window', color: 'var(--color-chart-1)' },
  { key: 'sevenDay', label: 'Weekly window', color: 'var(--color-chart-2)' },
] as const

const RANGE_MS: Record<Range, number> = { '24h': 86_400_000, '7d': 7 * 86_400_000, '30d': 30 * 86_400_000 }

export function QuotaLegend() {
  return (
    <div className="legend">
      {QUOTA_SERIES.map((series) => (
        <span className="legend-item" key={series.key}>
          <span className="legend-swatch" style={{ background: series.color }} />
          {series.label}
        </span>
      ))}
    </div>
  )
}

/** Utilization of both windows over the range, on a fixed 0–100% axis. */
export function QuotaChart({ points, range }: { points: QuotaPoint[]; range: Range }) {
  const end = points.length > 0 ? new Date(points[points.length - 1].time).getTime() : 0
  const start = end - RANGE_MS[range]
  const x = (time: string) => Math.min(100, Math.max(0, ((new Date(time).getTime() - start) / (end - start)) * 100))
  const ticks = [1, 0.75, 0.5, 0.25, 0]

  const path = (key: 'fiveHour' | 'sevenDay') => {
    let d = ''
    for (const point of points) {
      const value = point[key]
      if (value === null) continue
      d += `${d === '' ? 'M' : 'L'}${x(point.time).toFixed(2)},${((1 - Math.min(1, value)) * 100).toFixed(2)} `
    }
    return d
  }

  const labels = [0, 0.25, 0.5, 0.75, 1].map((fraction) => ({
    fraction,
    time: new Date(start + fraction * RANGE_MS[range]).toISOString(),
  }))

  return (
    <div className="chart">
      <div className="yaxis" aria-hidden="true">
        {ticks.map((tick) => (
          <span key={tick}>{Math.round(tick * 100)}%</span>
        ))}
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="plot plot--lines">
          <div className="grid-line" style={{ top: '25%' }} />
          <div className="grid-line" style={{ top: '50%' }} />
          <div className="grid-line" style={{ top: '75%' }} />
          <svg className="lines" viewBox="0 0 100 100" preserveAspectRatio="none" role="img" aria-label="Quota utilization over time">
            {QUOTA_SERIES.map((series) => (
              <path
                key={series.key}
                d={path(series.key)}
                fill="none"
                stroke={series.color}
                strokeWidth={2}
                strokeLinejoin="round"
                vectorEffect="non-scaling-stroke"
              />
            ))}
          </svg>
        </div>
        <div className="xaxis">
          {labels.map((label) => (
            <span key={label.fraction} style={{ left: `${label.fraction * 100}%` }}>
              {formatBucketLabel(label.time, range)}
            </span>
          ))}
        </div>
      </div>
    </div>
  )
}
