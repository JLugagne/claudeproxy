import type { Range } from '../api'
import { useLiveStatus } from '../live'
import type { Theme } from '../theme'

const ranges: { value: Range; label: string }[] = [
  { value: '24h', label: '24H' },
  { value: '7d', label: '7D' },
  { value: '30d', label: '30D' },
]

type Props = {
  title: string
  subtitle: string
  theme: Theme
  onToggleTheme: () => void
  range: Range
  onRangeChange: (range: Range) => void
}

export function PageHeader({ title, subtitle, theme, onToggleTheme, range, onRangeChange }: Props) {
  const live = useLiveStatus()

  return (
    <header className="page-header">
      <div>
        <h1 className="page-title">{title}</h1>
        <p className="page-sub">{subtitle}</p>
      </div>

      <div className="page-header-right">
        {live !== 'off' && (
          <span className="live" data-status={live} role="status" aria-live="polite">
            <span className="live-dot" />
            {live === 'live' ? 'Live' : 'Connecting'}
          </span>
        )}
        <div className="range" role="group" aria-label="Time range">
          {ranges.map((item) => (
            <button
              key={item.value}
              type="button"
              className="range-btn"
              data-active={range === item.value}
              aria-pressed={range === item.value}
              onClick={() => onRangeChange(item.value)}
            >
              {item.label}
            </button>
          ))}
        </div>
        <button
          type="button"
          className="theme-btn"
          onClick={onToggleTheme}
          aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`}
        >
          {theme === 'dark' ? 'Light' : 'Dark'}
        </button>
      </div>
    </header>
  )
}
