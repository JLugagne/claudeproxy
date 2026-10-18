import type { ReactNode } from 'react'
import type { Page } from '../router'
import { hrefFor, useNavigate } from '../router'

type Props = {
  page: Page
  className?: string
  active?: boolean
  children: ReactNode
}

/** An in-app link: a real href that a fresh tab or a middle click can follow,
 *  intercepted for a client-side transition. */
export function NavLink({ page, className, active, children }: Props) {
  const navigate = useNavigate()
  return (
    <a
      href={hrefFor(page)}
      className={className}
      data-active={active}
      aria-current={active ? 'page' : undefined}
      onClick={(event) => {
        if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
        event.preventDefault()
        navigate(page)
      }}
    >
      {children}
    </a>
  )
}
