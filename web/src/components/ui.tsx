import type { CSSProperties, ReactNode } from 'react'
import type { Stat } from '../stats'
import { cx } from '../util'

export function Stats({ items }: { items: Stat[] }) {
  return (
    <div className="stats">
      {items.map((stat) => (
        <div className="stat" key={stat.label}>
          <p className="stat-label">{stat.label}</p>
          <p className="stat-value">{stat.value}</p>
          <p className={cx('stat-sub', stat.tone === 'success' && 'stat-sub--success')}>{stat.sub}</p>
        </div>
      ))}
    </div>
  )
}

export function Panel({
  children,
  grow,
  className,
}: {
  children: ReactNode
  grow?: boolean
  className?: string
}) {
  return <section className={cx('panel', grow && 'panel--grow', className)}>{children}</section>
}

export function PanelHead({
  title,
  subtitle,
  action,
}: {
  title: string
  subtitle?: string
  action?: ReactNode
}) {
  return (
    <div className="panel-head">
      <div>
        <h2 className="panel-title">{title}</h2>
        {subtitle ? <p className="panel-sub">{subtitle}</p> : null}
      </div>
      {action}
    </div>
  )
}

export function Th({
  w,
  grow,
  right,
  className,
  children,
}: {
  w?: number
  grow?: boolean
  right?: boolean
  className?: string
  children: ReactNode
}) {
  return (
    <div
      className={cx('th', grow && 'grow', right && 'right', className)}
      style={w !== undefined ? { width: w } : undefined}
    >
      {children}
    </div>
  )
}

export function Cell({
  w,
  grow,
  right,
  className,
  style,
  children,
}: {
  w?: number
  grow?: boolean
  right?: boolean
  className?: string
  style?: CSSProperties
  children?: ReactNode
}) {
  return (
    <div
      className={cx('tcell', grow && 'grow', right && 'right', className)}
      style={{ ...(w !== undefined ? { width: w } : {}), ...style }}
    >
      {children}
    </div>
  )
}

export function Pill({
  tone = 'success',
  mono,
  children,
}: {
  tone?: 'success' | 'warning' | 'muted'
  mono?: boolean
  children: ReactNode
}) {
  return <span className={cx('pill', `pill--${tone}`, mono && 'mono')}>{children}</span>
}

export type BarRowView = {
  name: string
  percent: number
  value: string
  color: number
}

export function BarRowList({ rows }: { rows: BarRowView[] }) {
  return (
    <div>
      {rows.map((row) => (
        <div className="barrow" key={row.name}>
          <span
            className="row-dot"
            style={{ background: `var(--color-chart-${((row.color - 1) % 5) + 1})` }}
            aria-hidden="true"
          />
          <span className="barrow-name">{row.name}</span>
          <div className="track">
            <div
              className="track-fill"
              style={{
                width: `${Math.max(row.percent, 0)}%`,
                background: `var(--color-chart-${((row.color - 1) % 5) + 1})`,
              }}
            />
          </div>
          <span className="barrow-value">{row.value}</span>
        </div>
      ))}
    </div>
  )
}

export function NoteBar({
  label,
  children,
  action,
}: {
  label: string
  children: ReactNode
  action?: ReactNode
}) {
  return (
    <div className="note">
      <span className="note-label">{label}</span>
      <span className="note-text">{children}</span>
      {action}
    </div>
  )
}

export function MessagePanel({ title, message }: { title: string; message: string }) {
  return (
    <Panel>
      <div className="message">
        <h2 className="panel-title">{title}</h2>
        <p className="panel-sub">{message}</p>
      </div>
    </Panel>
  )
}

/** The whole-page state before any data is available. */
export function PageStatus({ error }: { error: string | null }) {
  if (error) return <MessagePanel title="Metrics unavailable" message={error} />
  return <MessagePanel title="Loading" message="Reading the usage store…" />
}

export function EmptyRow({ message }: { message: string }) {
  return (
    <div className="trow trow--last">
      <div className="tcell grow faint cell-sm">{message}</div>
    </div>
  )
}
