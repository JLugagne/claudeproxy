import { useEffect, useState } from 'react'
import { apiGet } from './api'
import { subscribeLive } from './live'

const LIVE_REFRESH_MS = 2000

type State<T> = {
  data: T | null
  error: string | null
  loading: boolean
}

export function useApi<T>(path: string): State<T> {
  const [state, setState] = useState<State<T>>({ data: null, error: null, loading: true })

  useEffect(() => {
    const controller = new AbortController()
    setState({ data: null, error: null, loading: true })

    apiGet<T>(path, controller.signal)
      .then((data) => setState({ data, error: null, loading: false }))
      .catch((error: unknown) => {
        if (controller.signal.aborted) return
        setState({
          data: null,
          error: error instanceof Error ? error.message : String(error),
          loading: false,
        })
      })

    return () => controller.abort()
  }, [path])

  // Live refresh: refetch silently, at most once every LIVE_REFRESH_MS, with a
  // trailing call so a burst of events still ends on fresh data.
  useEffect(() => {
    let timer: number | null = null
    let pending = false
    let inflight: AbortController | null = null

    const refetch = () => {
      inflight?.abort()
      const controller = new AbortController()
      inflight = controller
      apiGet<T>(path, controller.signal)
        .then((data) => {
          if (controller.signal.aborted) return
          setState({ data, error: null, loading: false })
        })
        .catch(() => {
          // keep the current data; a live refresh never surfaces an error
        })
    }

    const endWindow = () => {
      timer = null
      if (!pending) return
      pending = false
      refetch()
      timer = window.setTimeout(endWindow, LIVE_REFRESH_MS)
    }

    const schedule = () => {
      if (timer !== null) {
        pending = true
        return
      }
      refetch()
      timer = window.setTimeout(endWindow, LIVE_REFRESH_MS)
    }

    const unsubscribe = subscribeLive(schedule)

    return () => {
      unsubscribe()
      if (timer !== null) window.clearTimeout(timer)
      inflight?.abort()
    }
  }, [path])

  return state
}
