import { useSyncExternalStore } from 'react'
import type { RequestRow } from './api'

export type LiveEvent = { type: 'request'; row: RequestRow } | { type: 'resync' }

export type LiveStatus = 'live' | 'connecting' | 'off'

type Listener = (event: LiveEvent) => void

const listeners = new Set<Listener>()
const statusListeners = new Set<() => void>()

let source: EventSource | null = null
let subscribers = 0
let status: LiveStatus = 'off'
let sawError = false

function setStatus(next: LiveStatus) {
  if (status === next) return
  status = next
  for (const notify of [...statusListeners]) notify()
}

function notify(event: LiveEvent) {
  for (const listener of [...listeners]) {
    try {
      listener(event)
    } catch {
      // a failing listener must not stop the others
    }
  }
}

function open() {
  const events = new EventSource('/dashboard/api/events')
  source = events
  sawError = false
  setStatus('connecting')

  events.addEventListener('open', () => {
    setStatus('live')
    if (sawError) {
      sawError = false
      notify({ type: 'resync' })
    }
  })

  events.addEventListener('error', () => {
    if (events.readyState === EventSource.CLOSED) {
      // The browser gave up (e.g. the endpoint answered 404).
      setStatus('off')
      return
    }
    // The browser reconnects on its own; a later `open` is a resync.
    sawError = true
    setStatus('connecting')
  })

  events.addEventListener('request', (event: MessageEvent<string>) => {
    let row: unknown
    try {
      row = JSON.parse(event.data)
    } catch {
      return
    }
    if (typeof row !== 'object' || row === null || Array.isArray(row)) return
    notify({ type: 'request', row: row as RequestRow })
  })
}

function close() {
  const events = source
  source = null
  sawError = false
  setStatus('off')
  events?.close()
}

function acquire() {
  subscribers += 1
  if (subscribers === 1) open()
}

function release() {
  subscribers -= 1
  if (subscribers === 0) close()
}

export function subscribeLive(listener: Listener): () => void {
  listeners.add(listener)
  acquire()
  let active = true
  return () => {
    if (!active) return
    active = false
    listeners.delete(listener)
    release()
  }
}

function subscribeStatus(onChange: () => void): () => void {
  statusListeners.add(onChange)
  acquire()
  return () => {
    statusListeners.delete(onChange)
    release()
  }
}

function getStatus(): LiveStatus {
  return status
}

/** Current state of the shared event stream. */
export function useLiveStatus(): LiveStatus {
  return useSyncExternalStore(subscribeStatus, getStatus, getStatus)
}
