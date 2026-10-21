import { useState } from 'react'
import type { ReactElement } from 'react'
import type { Range } from './api'
import { PageHeader } from './components/PageHeader'
import { Sidebar } from './components/Sidebar'
import { Models } from './pages/Models'
import { Overview } from './pages/Overview'
import { Projects } from './pages/Projects'
import { Providers } from './pages/Providers'
import { Quota } from './pages/Quota'
import { Requests } from './pages/Requests'
import { Sessions } from './pages/Sessions'
import type { Page } from './router'
import { NavigateContext, pageMeta, usePage } from './router'
import { useTheme } from './theme'

const pages: Record<Page, (props: { range: Range }) => ReactElement> = {
  overview: Overview,
  projects: Projects,
  models: Models,
  requests: Requests,
  sessions: Sessions,
  quota: Quota,
  providers: Providers,
}

export default function App() {
  const [page, navigate] = usePage()
  const [theme, toggleTheme] = useTheme()
  const [range, setRange] = useState<Range>('24h')

  const meta = pageMeta[page]
  const Current = pages[page]

  return (
    <NavigateContext.Provider value={navigate}>
      <div className="app">
        <Sidebar page={page} />
        <main className="main">
          <PageHeader
            title={meta.title}
            subtitle={meta.subtitle}
            theme={theme}
            onToggleTheme={toggleTheme}
            range={range}
            onRangeChange={setRange}
          />
          <div className="content">
            <Current range={range} />
          </div>
        </main>
      </div>
    </NavigateContext.Provider>
  )
}
