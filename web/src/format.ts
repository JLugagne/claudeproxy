import type { Range } from './api'

const countFormat = new Intl.NumberFormat('en-US')

export function formatCount(value: number): string {
  return countFormat.format(value)
}

function trimmed(value: number): string {
  if (value >= 100) return value.toFixed(0)
  if (value >= 10) return value.toFixed(1)
  return value.toFixed(2)
}

export function formatCompact(value: number): string {
  if (value >= 1_000_000) return `${trimmed(value / 1_000_000)}M`
  if (value >= 1_000) return `${trimmed(value / 1_000)}k`
  return String(Math.round(value))
}

export function formatPercent(ratio: number): string {
  return `${(ratio * 100).toFixed(1)}%`
}

export function formatShare(part: number, total: number): string {
  if (total <= 0) return '0.0%'
  return formatPercent(part / total)
}

export function formatLatency(ms: number): string {
  if (ms <= 0) return '—'
  return `${(ms / 1000).toFixed(2)}s`
}

export function formatDuration(ms: number): string {
  const totalMinutes = Math.round(ms / 60_000)
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return `${hours}h ${String(minutes).padStart(2, '0')}m`
}

/** A price the provider reported; a dash when it reported none. */
export function formatCost(usd: number): string {
  if (usd <= 0) return '—'
  if (usd < 0.01) return `$${usd.toFixed(4)}`
  return `$${usd.toFixed(2)}`
}

export function formatClock(iso: string): string {
  return new Date(iso).toLocaleTimeString('en-GB', { hour12: false })
}

export function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
}

/** A window's utilization, zero once its reset time has passed: the provider
 *  only reports a window with the next response. */
export function effectiveUtilization(window: { utilization: number; resetsAt: string | null }, now = Date.now()): number {
  if (window.resetsAt && new Date(window.resetsAt).getTime() <= now) return 0
  return window.utilization
}

export function formatWholePercent(ratio: number): string {
  return `${Math.round(ratio * 100)}%`
}

/** Time left before a reset, e.g. "2h 05m" or "3d 4h". */
export function formatTimeLeft(iso: string | null, now = Date.now()): string {
  if (!iso) return '—'
  const minutes = Math.max(0, Math.round((new Date(iso).getTime() - now) / 60_000))
  if (minutes >= 24 * 60) return `${Math.floor(minutes / (24 * 60))}d ${Math.floor((minutes % (24 * 60)) / 60)}h`
  return `${Math.floor(minutes / 60)}h ${String(minutes % 60).padStart(2, '0')}m`
}

export function rangeLabel(range: Range): string {
  switch (range) {
    case '7d':
      return 'Last 7 days'
    case '30d':
      return 'Last 30 days'
    default:
      return 'Last 24 hours'
  }
}

export function formatBucketLabel(iso: string, range: Range): string {
  const date = new Date(iso)
  if (range === '24h') {
    return `${String(date.getHours()).padStart(2, '0')}:00`
  }
  return date.toLocaleDateString('en-GB', { day: '2-digit', month: 'short' })
}

/** Round a maximum up to a readable axis bound (1, 2 or 5 × 10^n). */
export function niceMax(value: number): number {
  if (value <= 0) return 1
  const magnitude = 10 ** Math.floor(Math.log10(value))
  const normalized = value / magnitude
  const step = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10
  return step * magnitude
}
