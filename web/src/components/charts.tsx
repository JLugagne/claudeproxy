import type { Point, Range } from '../api'
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
