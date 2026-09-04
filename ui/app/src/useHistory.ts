import { useEffect, useState } from 'react'
import type { HistoryRange, HistoryResponse } from './types'

export interface UseHistoryResult {
  data: HistoryResponse | null
  loading: boolean
  error: string | null
}

/** Polls GET /api/v1/history?range=... on an interval, tracking loading/error/data states
 * explicitly so callers can render real empty/error states instead of silently showing stale or
 * zeroed-out numbers. Re-fetches immediately when `range` changes. */
export function useHistory(range: HistoryRange, pollMs = 30_000): UseHistoryResult {
  const [data, setData] = useState<HistoryResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)

    async function fetchOnce() {
      try {
        const res = await fetch(`/api/v1/history?range=${encodeURIComponent(range)}`)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const json = (await res.json()) as HistoryResponse
        if (!cancelled) {
          setData(json)
          setLoading(false)
        }
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : String(e))
          setLoading(false)
        }
      }
    }

    fetchOnce()
    const id = setInterval(fetchOnce, pollMs)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [range, pollMs])

  return { data, loading, error }
}
