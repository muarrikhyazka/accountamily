import { createContext, useContext, useEffect, useRef, useState } from 'react'
import { api } from './api'

const OfflineContext = createContext(null)
const QUEUE_KEY = 'accountamily_offline_queue'

function loadQueue() {
  try { return JSON.parse(localStorage.getItem(QUEUE_KEY) || '[]') } catch { return [] }
}
function saveQueue(q) {
  localStorage.setItem(QUEUE_KEY, JSON.stringify(q))
}
function uuid() {
  return crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

export function OfflineProvider({ children }) {
  const [serverOnline, setServerOnline] = useState(true)
  const [queueSize, setQueueSize] = useState(loadQueue().length)
  const syncing = useRef(false)

  async function checkServer() {
    if (!navigator.onLine) { setServerOnline(false); return false }
    try {
      await api.ping()
      setServerOnline(true)
      return true
    } catch {
      setServerOnline(false)
      return false
    }
  }

  async function flushQueue() {
    if (syncing.current) return
    syncing.current = true
    try {
      let queue = loadQueue()
      for (const item of [...queue]) {
        try {
          await api.addSpending(item)
          queue = queue.filter(q => q.client_id !== item.client_id)
          saveQueue(queue)
          setQueueSize(queue.length)
        } catch {
          break // stop on first failure, retry next tick
        }
      }
    } finally {
      syncing.current = false
    }
  }

  useEffect(() => {
    let mounted = true
    async function tick() {
      const online = await checkServer()
      if (online && mounted) await flushQueue()
    }
    tick()
    const interval = setInterval(tick, 15000)
    window.addEventListener('online', tick)
    window.addEventListener('offline', () => setServerOnline(false))
    return () => {
      mounted = false
      clearInterval(interval)
      window.removeEventListener('online', tick)
    }
  }, [])

  // Wrap addSpending: try live, fall back to local queue on failure.
  async function addSpendingResilient(payload) {
    const withId = { ...payload, client_id: payload.client_id || uuid() }
    if (serverOnline) {
      try {
        return await api.addSpending(withId)
      } catch {
        // fall through to queue
      }
    }
    const queue = loadQueue()
    queue.push(withId)
    saveQueue(queue)
    setQueueSize(queue.length)
    return { ok: true, queued: true }
  }

  return (
    <OfflineContext.Provider value={{ serverOnline, queueSize, addSpendingResilient }}>
      {children}
    </OfflineContext.Provider>
  )
}

export function useOffline() {
  return useContext(OfflineContext)
}
