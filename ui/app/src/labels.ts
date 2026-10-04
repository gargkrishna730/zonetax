import type { MapRange, MapResponse } from './types'

export const RANGE_LABEL: Record<MapRange, string> = {
  '15m': 'last 15 minutes',
  '1h': 'last hour',
  '6h': 'last 6 hours',
  '24h': 'last 24 hours',
  '7d': 'last 7 days',
  custom: 'custom range',
}

export type DataStatus = 'loading' | 'error' | 'stale' | 'incomplete' | 'measured'

export function dataStatusOf(data: MapResponse | null, loading: boolean, error: string | null): DataStatus {
  if (error) return 'error'
  if (loading && !data) return 'loading'
  if (data && !data.has_data) return 'stale'
  if (data && !data.complete) return 'incomplete'
  return 'measured'
}
