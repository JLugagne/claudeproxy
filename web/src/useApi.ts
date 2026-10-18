import { useEffect, useState } from 'react'
import { apiGet } from './api'

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

  return state
}
