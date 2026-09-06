import { useCallback, useEffect, useState } from 'react'
import type { MapRange, MapResponse } from './types'

export interface UseMapDataResult {
  data: MapResponse | null
  loading: boolean
  error: string | null
  refetch: () => void
}

/** Polls GET /api/v1/map?range=... — the Cross-AZ Service Map's primary data source, giving
 * REAL per-route cost/traffic for the selected time window (unlike /api/v1/costs, which only
 * ever reports cumulative-since-restart totals). Re-fetches immediately when range or the
 * custom since/until bounds change, so switching time ranges genuinely rebuilds the map rather
 * than just relabeling the same numbers. `refetch()` lets the toolbar's manual Refresh button
 * trigger an immediate fetch without waiting for the next poll tick. */
export function useMapData(range: MapRange, customSince: string | null, customUntil: string | null, pollMs = 15_000): UseMapDataResult {
  const [data, setData] = useState<MapResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [refetchNonce, setRefetchNonce] = useState(0)

  useEffect(() => {
    if (range === 'custom' && (!customSince || !customUntil)) {
      // Incomplete custom range — don't fetch a request the API would 400 on; the UI shows an
      // explicit "pick a start and end" state instead of erroring against the backend.
      setLoading(false)
      setError(null)
      return
    }
    let cancelled = false
    setLoading(true)
    setError(null)

    async function fetchOnce() {
      try {
        const params = new URLSearchParams({ range })
        if (range === 'custom' && customSince && customUntil) {
          params.set('since', customSince)
          params.set('until', customUntil)
        }
        const res = await fetch(`/api/v1/map?${params.toString()}`, { cache: 'no-store' })
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const json = (await res.json()) as MapResponse
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
    const id = pollMs > 0 && Number.isFinite(pollMs) ? setInterval(fetchOnce, pollMs) : null
    return () => {
      cancelled = true
      if (id) clearInterval(id)
    }
  }, [range, customSince, customUntil, pollMs, refetchNonce])

  const refetch = useCallback(() => setRefetchNonce((n) => n + 1), [])

  return { data, loading, error, refetch }
}
