import type { Page } from '../router'
import { NavLink } from './NavLink'
import { QuotaMeters } from './QuotaMeters'

const analyticsNav: { page: Page; label: string }[] = [
  { page: 'overview', label: 'Overview' },
  { page: 'projects', label: 'Projects' },
  { page: 'models', label: 'Models' },
  { page: 'requests', label: 'Requests' },
  { page: 'sessions', label: 'Sessions' },
  { page: 'quota', label: 'Quota' },
]

const systemNav: { page: Page; label: string }[] = [{ page: 'providers', label: 'Providers' }]

export function Sidebar({ page }: { page: Page }) {
  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark" aria-hidden="true" />
        <div className="brand-text">
          <span className="brand-name">claudeproxy</span>
          <span className="brand-sub">usage console</span>
        </div>
      </div>

      <div className="nav-label">Analytics</div>
      <nav className="nav">
        {analyticsNav.map((item) => (
          <NavLink key={item.page} page={item.page} className="nav-item" active={page === item.page}>
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div className="nav-label">System</div>
      <nav className="nav">
        {systemNav.map((item) => (
          <NavLink key={item.page} page={item.page} className="nav-item" active={page === item.page}>
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div className="sidebar-footer">
        <QuotaMeters />
        <div className="status">
          <span className="dot" aria-hidden="true" />
          Proxy online
        </div>
        <span className="addr">127.0.0.1:8787</span>
      </div>
    </aside>
  )
}
