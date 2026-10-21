import { createContext, useContext, useEffect, useState } from 'react'

export const PAGES = ['overview', 'projects', 'models', 'requests', 'sessions', 'quota', 'providers'] as const

export type Page = (typeof PAGES)[number]

const BASE = '/dashboard'

export const pageMeta: Record<Page, { title: string; subtitle: string }> = {
  overview: { title: 'Overview', subtitle: 'All traffic through the proxy' },
  projects: { title: 'Projects', subtitle: 'Usage attributed to each git project' },
  models: { title: 'Models', subtitle: 'Routing and consumption per upstream model' },
  requests: { title: 'Requests', subtitle: 'Every message routed through the proxy' },
  sessions: { title: 'Sessions', subtitle: 'Claude Code sessions and their consumption' },
  quota: { title: 'Quota', subtitle: 'Subscription usage over the 5-hour and weekly windows' },
  providers: { title: 'Providers', subtitle: 'Upstreams, credentials and routing' },
}

export function hrefFor(page: Page): string {
  return page === 'overview' ? `${BASE}/` : `${BASE}/${page}`
}

function pageFromLocation(): Page {
  const last = window.location.pathname.replace(/\/+$/, '').split('/').pop() ?? ''
  return (PAGES as readonly string[]).includes(last) ? (last as Page) : 'overview'
}

export function usePage(): [Page, (page: Page) => void] {
  const [page, setPage] = useState<Page>(pageFromLocation)

  useEffect(() => {
    const onPop = () => setPage(pageFromLocation())
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])

  const navigate = (next: Page) => {
    if (next === page) return
    window.history.pushState({}, '', hrefFor(next))
    setPage(next)
    window.scrollTo(0, 0)
  }

  return [page, navigate]
}

export const NavigateContext = createContext<(page: Page) => void>(() => {})

export function useNavigate(): (page: Page) => void {
  return useContext(NavigateContext)
}
