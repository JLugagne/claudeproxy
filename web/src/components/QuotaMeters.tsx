import type { QuotaResponse, QuotaWindow } from '../api'
import { effectiveUtilization, formatTimeLeft, formatWholePercent } from '../format'
import { useApi } from '../useApi'

export function quotaTone(ratio: number): 'ok' | 'warn' | 'high' {
  if (ratio >= 0.9) return 'high'
  if (ratio >= 0.7) return 'warn'
  return 'ok'
}

export function QuotaMeter({ label, window }: { label: string; window: QuotaWindow | null }) {
  const known = window !== null && window.present
  const ratio = known ? effectiveUtilization(window) : 0
  const percent = Math.min(100, Math.max(0, ratio * 100))

  return (
    <div className="meter">
      <div className="meter-head">
        <span className="meter-label">{label}</span>
        <span className="meter-value">{known ? formatWholePercent(ratio) : '—'}</span>
      </div>
      <div
        className="meter-track"
        role="progressbar"
        aria-label={`${label} usage`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(percent)}
      >
        <div className={`meter-fill meter-fill--${quotaTone(ratio)}`} style={{ width: `${percent}%` }} />
      </div>
      <span className="meter-sub">{known ? `resets in ${formatTimeLeft(window.resetsAt)}` : 'no data yet'}</span>
    </div>
  )
}

/** The two subscription windows as compact progress bars, for the sidebar. */
export function QuotaMeters() {
  const { data } = useApi<QuotaResponse>('/quota?range=7d')
  const current = data?.current ?? null

  return (
    <div className="meters">
      <QuotaMeter label="5h usage" window={current?.fiveHour ?? null} />
      <QuotaMeter label="Weekly usage" window={current?.sevenDay ?? null} />
    </div>
  )
}
